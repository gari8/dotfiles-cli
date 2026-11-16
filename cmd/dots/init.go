package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "init [<dotfiles dir>]",
		Short: "Register the dotfiles repository, create dots.toml and link files",
		Long: `Register the dotfiles repository (default: the current directory) in
~/.config/dots/config.toml, create dots.toml if it is missing, and create the
symlinks listed under [links]. Safe to run again; existing files are backed up.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return initRepo(dir, dryRun)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "only show what would be done")
	return cmd
}

func initRepo(dir string, dryRun bool) error {
	dir, err := expandHome(dir)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return fmt.Errorf("not a directory: %s", tildePath(root))
	}
	if dryRun {
		fmt.Println("(dry run: nothing is changed)")
	}

	if err := registerRoot(root, dryRun); err != nil {
		return err
	}

	// Create dots.toml from the template, or check the existing one.
	cfgPath := filepath.Join(root, configFile)
	if _, err := os.Stat(cfgPath); errors.Is(err, fs.ErrNotExist) {
		fmt.Printf("  create  %s\n", tildePath(cfgPath))
		if !dryRun {
			if err := os.WriteFile(cfgPath, []byte(configTemplate), 0o644); err != nil {
				return err
			}
		}
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	return applyLinks(root, cfg.Links, dryRun)
}

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

# Paths relative to this repository.
[paths]
brewfile = "Brewfile"
mise_config = "config/mise/config.toml"
install = "scripts/install.sh"

# dots add puts new Brewfile entries at the end of the section whose heading
# ("# --- ... ---") contains this text; without a match they go at the end.
[brewfile]
formula_section = "CLI"
cask_section = "GUI"

# Symlinks created by dots init: "file in this repository" = "link target".
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
