package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

// initPaths are the --brewfile-cli / --brewfile-gui / --mise-config values
// (empty when not given).
type initPaths struct {
	brewfileCLI, brewfileGUI, miseConfig string
}

func newInitCmd() *cobra.Command {
	var dryRun bool
	var from string
	var paths initPaths
	cmd := &cobra.Command{
		Use:   "init [<dotfiles dir>]",
		Short: "Register the dotfiles repository, create missing config files and link files",
		Long: `Register the dotfiles repository (default: ~/dotfiles; created as a new git
repository after asking if it does not exist) in
~/.config/dots/config.toml, create dots.toml, the CLI and GUI Brewfiles and the
mise config if they are missing, and create the symlinks listed under [links].
Safe to run again; existing files are backed up.

--from clones a git repository into the directory first (it must not exist).

--brewfile-cli, --brewfile-gui and --mise-config take either
  a local file (relative to the current directory): recorded under [paths]
  <git url>//<path>[?ref=<ref>]: copied into this repository (asks before
                                 replacing an existing file)`,
		Example: `  dots init --from https://github.com/you/dotfiles.git   # no login needed for a public repository
  dots init --from git@github.com:you/dotfiles.git       # with an SSH key added to GitHub
  dots init --brewfile-gui ./Brewfile --mise-config ./mise.toml
  dots init --brewfile-gui https://github.com/someone/dotfiles.git//brew/gui.Brewfile`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "~/dotfiles"
			if len(args) == 1 {
				dir = args[0]
			}
			if from != "" {
				if err := cloneRepo(from, dir, dryRun); err != nil || dryRun {
					return err
				}
			}
			return initRepo(dir, paths, dryRun)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "only show what would be done")
	cmd.Flags().StringVar(&from, "from", "", "git repository to clone into the directory first")
	cmd.Flags().StringVar(&paths.brewfileCLI, "brewfile-cli", "", "Brewfile for formulae (sets paths.brewfile_cli)")
	cmd.Flags().StringVar(&paths.brewfileGUI, "brewfile-gui", "", "Brewfile for casks and mas apps (sets paths.brewfile_gui)")
	cmd.Flags().StringVar(&paths.miseConfig, "mise-config", "", "mise config to use (sets paths.mise_config)")
	return cmd
}

func initRepo(dir string, paths initPaths, dryRun bool) error {
	dir, err := expandHome(dir)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Println("(dry run: nothing is changed)")
	}
	if fi, err := os.Stat(root); err == nil && !fi.IsDir() {
		return fmt.Errorf("not a directory: %s", tildePath(root))
	} else if err != nil {
		// Starting from scratch: make a new repository.
		if err := createRepo(root, dryRun); err != nil || dryRun {
			return err
		}
	} else if !succeeds(root, "git", "rev-parse", "--is-inside-work-tree") {
		// An existing directory that is not under git yet (custom commands
		// and syncing expect a repository).
		fmt.Printf("  git     init %s\n", tildePath(root))
		if !dryRun {
			if err := run(root, "git", "init", "--quiet"); err != nil {
				return err
			}
		}
	}

	if err := registerRoot(root, dryRun); err != nil {
		return err
	}

	// Create dots.toml from the template, or check the existing one.
	cfgPath := filepath.Join(root, configFile)
	if err := createIfMissing(cfgPath, configTemplate, dryRun); err != nil {
		return err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	// Local files are recorded under [paths]; remote ones are copied in below.
	local, remote := splitPaths(paths)
	if err := setPaths(root, cfgPath, &cfg, local, dryRun); err != nil {
		return err
	}

	// GUI apps come from Homebrew and CLI tools from mise, so both files are needed.
	r := &repo{root: root, cfg: cfg}
	defaults := defaultConfig().Paths
	imports := []struct{ value, defaultPath, dest string }{
		{remote.brewfileCLI, defaults.BrewfileCLI, r.brewfileFor(formula)},
		{remote.brewfileGUI, defaults.BrewfileGUI, r.brewfileFor(cask)},
		{remote.miseConfig, defaults.MiseConfig, r.miseConfig()},
	}
	for _, im := range imports {
		if im.value == "" {
			continue
		}
		if err := importFile(parseRemote(im.value, im.defaultPath), im.dest, dryRun); err != nil {
			return err
		}
	}
	if err := createIfMissing(r.brewfileFor(formula), brewfileCLITemplate, dryRun); err != nil {
		return err
	}
	if err := createIfMissing(r.brewfileFor(cask), brewfileGUITemplate, dryRun); err != nil {
		return err
	}
	if err := createIfMissing(r.miseConfig(), miseTemplate, dryRun); err != nil {
		return err
	}

	return applyLinks(root, cfg.Links, dryRun)
}

// createRepo makes an empty git repository at root, after asking.
func createRepo(root string, dryRun bool) error {
	fmt.Printf("  create  %s (new git repository)\n", tildePath(root))
	if dryRun {
		fmt.Println("(dry run: stopping here; to use an existing repository, pass --from <git url>)")
		return nil
	}
	if !confirm(fmt.Sprintf("%s does not exist. Create a new dotfiles repository there?", tildePath(root))) {
		return fmt.Errorf("canceled; to use an existing repository: dots init --from <git url>")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return run(root, "git", "init", "--quiet")
}

// cloneRepo clones repo into dir, which must not exist or be empty.
func cloneRepo(repo, dir string, dryRun bool) error {
	dir, err := expandHome(dir)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s already exists; drop --from, or give another directory", tildePath(dir))
	}
	fmt.Printf("  clone   %s -> %s\n", repo, tildePath(dir))
	if dryRun {
		fmt.Println("(dry run: stopping here, the rest needs the clone)")
		return nil
	}
	return gitClone(repo, "", dir, false)
}

// splitPaths separates local paths from <git url>//<path> values.
func splitPaths(p initPaths) (local, remote initPaths) {
	split := func(v string, l, r *string) {
		if isRemote(v) {
			*r = v
		} else {
			*l = v
		}
	}
	split(p.brewfileCLI, &local.brewfileCLI, &remote.brewfileCLI)
	split(p.brewfileGUI, &local.brewfileGUI, &remote.brewfileGUI)
	split(p.miseConfig, &local.miseConfig, &remote.miseConfig)
	return local, remote
}

// importFile copies a file from a git repository to dest, asking before
// replacing an existing file.
func importFile(spec remoteSpec, dest string, dryRun bool) error {
	fmt.Printf("  copy    %s -> %s\n", spec, tildePath(dest))
	if dryRun {
		return nil
	}
	content, err := fetchRemote(spec)
	if err != nil {
		return err
	}
	if exists(dest) && !confirm(fmt.Sprintf("Replace %s?", tildePath(dest))) {
		fmt.Printf("  keep    %s\n", tildePath(dest))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, content, 0o644)
}

// setPaths records --brewfile-cli / --brewfile-gui / --mise-config under [paths] in dots.toml and
// in cfg. Paths inside the repository are stored relative to it.
func setPaths(root, cfgPath string, cfg *repoConfig, paths initPaths, dryRun bool) error {
	sets := []struct {
		key, value string
		field      *string
	}{
		{"brewfile_cli", paths.brewfileCLI, &cfg.Paths.BrewfileCLI},
		{"brewfile_gui", paths.brewfileGUI, &cfg.Paths.BrewfileGUI},
		{"mise_config", paths.miseConfig, &cfg.Paths.MiseConfig},
	}
	for _, s := range sets {
		if s.value == "" {
			continue
		}
		value, err := repoRelative(root, s.value)
		if err != nil {
			return err
		}
		*s.field = value
		fmt.Printf("  set     paths.%s = %q\n", s.key, value)
		if dryRun {
			continue
		}
		content, err := os.ReadFile(cfgPath)
		if err != nil {
			return err
		}
		updated := setTOMLKey(string(content), "paths", s.key, value)
		if err := os.WriteFile(cfgPath, []byte(updated), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// repoRelative resolves path (relative to the current directory, ~ allowed)
// and returns it relative to root when it is inside root, else absolute.
func repoRelative(root, path string) (string, error) {
	path, err := expandHome(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(root, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel, nil
	}
	return abs, nil
}

// setTOMLKey sets key = "value" in [section], keeping every other line and
// comment as it is. The key is added after the section header when missing,
// and the section is appended when it does not exist.
func setTOMLKey(content, section, key, value string) string {
	line := fmt.Sprintf("%s = %q", key, value)
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	keyLine := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=`)

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "["+section+"]" {
			start = i
			break
		}
	}
	if start < 0 {
		return strings.Join(lines, "\n") + "\n\n[" + section + "]\n" + line + "\n"
	}
	for i := start + 1; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "["); i++ {
		if keyLine.MatchString(lines[i]) {
			lines[i] = line
			return strings.Join(lines, "\n") + "\n"
		}
	}
	out := append([]string{}, lines[:start+1]...)
	out = append(out, line)
	out = append(out, lines[start+1:]...)
	return strings.Join(out, "\n") + "\n"
}

// createIfMissing writes content to path unless the file already exists.
func createIfMissing(path, content string, dryRun bool) error {
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	fmt.Printf("  create  %s\n", tildePath(path))
	if dryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// The Brewfiles that dots init creates.
const (
	brewfileCLITemplate = `# Homebrew formulae: system tools and libraries that mise does not provide.
# Edited by dots add --brew, applied by dots install.
`
	brewfileGUITemplate = `# Homebrew casks and Mac App Store apps (mas "Name", id: 123).
# Edited by dots add --cask, applied by dots install.
`
)

// miseTemplate is the mise config that dots init creates.
const miseTemplate = `# mise: CLI tools and language runtimes.
# Edited by dots add --mise, applied by dots install.
[tools]
`

// registerRoot writes root to the user config, asking before replacing a
// different one.
func registerRoot(root string, dryRun bool) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	uc, err := readUserConfig()
	if err != nil {
		return err
	}
	current, _ := expandHome(uc.Root)
	switch {
	case current == root:
		fmt.Printf("  ok      root %s\n", tildePath(root))
		return nil
	case current != "" && !dryRun:
		if !confirm(fmt.Sprintf("Change the dotfiles root from %s to %s?", tildePath(current), tildePath(root))) {
			return canceled("Init")
		}
	}
	fmt.Printf("  root    %s (%s)\n", tildePath(root), tildePath(path))
	if dryRun {
		return nil
	}

	uc.Root = root
	var buf bytes.Buffer
	buf.WriteString("# Written by dots init.\n")
	if err := toml.NewEncoder(&buf).Encode(uc); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// configTemplate is the dots.toml that dots init creates.
const configTemplate = `# dots configuration (https://github.com/gari8/dotfiles-cli)
version = 1

# Paths relative to this repository. GUI apps come from Homebrew and CLI tools
# from mise; the CLI Brewfile is for what mise does not provide.
[paths]
brewfile_cli = "brew/cli.Brewfile"   # formulae (dots add --brew)
brewfile_gui = "brew/gui.Brewfile"   # casks and mas apps (dots add --cask)
mise_config = "config/mise/config.toml"

# Symlinks created by dots init / dots install: "file in this repository" = "link target".
[links]
# "zsh/.zshrc" = "~/.zshrc"
# "config/mise" = "~/.config/mise"

# Custom commands: dots cmd <name> [args]
#   run:     shell command. A one-line command that does not use $1, "$@" etc.
#            gets the arguments appended; otherwise place them yourself.
#   desc:    shown by dots cmd list
#   dir:     working directory: root (this repository, default) or cwd
#   confirm: ask before running (skipped with -y)
# $DOTS_ROOT is this repository and $DOTS_PWD is where dots was run.
[cmd.status]
desc = "Show git status"
run = "git status -sb"
`
