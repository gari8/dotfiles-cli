package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// configVersion is the newest dots.toml format this build understands.
const configVersion = 1

// configFile is the per-repository config at the root of the dotfiles repository.
const configFile = "dots.toml"

// repoConfig is the contents of dots.toml. Every field has a default, so an
// empty or missing dots.toml works.
type repoConfig struct {
	Version int                  `toml:"version"`
	Include []string             `toml:"include"` // repo-relative files (globs) with more [cmd.*] tables
	Paths   pathsConfig          `toml:"paths"`
	Links   map[string]string    `toml:"links"` // repo-relative source -> target (~ allowed)
	Cmd     map[string]customCmd `toml:"cmd"`

	// cmdFrom is the repo-relative file each custom command is defined in.
	cmdFrom map[string]string
}

type pathsConfig struct {
	// GUI apps come from Homebrew; CLI tools from mise. The CLI Brewfile holds
	// the formulae mise does not provide (system tools, libraries), the GUI
	// Brewfile casks and mas apps. Both may point to the same file.
	BrewfileCLI string `toml:"brewfile_cli"`
	BrewfileGUI string `toml:"brewfile_gui"`
	MiseConfig  string `toml:"mise_config"`
}

func defaultConfig() repoConfig {
	return repoConfig{
		Version: configVersion,
		Paths: pathsConfig{
			BrewfileCLI: "brew/cli.Brewfile",
			BrewfileGUI: "brew/gui.Brewfile",
			MiseConfig:  "config/mise/config.toml",
		},
	}
}

// repo is a resolved dotfiles repository and its config.
type repo struct {
	root string
	cfg  repoConfig
}

// path returns a repo-relative path as an absolute one.
func (r *repo) path(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(r.root, rel)
}

func (r *repo) miseConfig() string { return r.path(r.cfg.Paths.MiseConfig) }

// brewfileFor is the Brewfile that dots add puts entries of kind k in.
func (r *repo) brewfileFor(k brewKind) string {
	if k == cask {
		return r.path(r.cfg.Paths.BrewfileGUI)
	}
	return r.path(r.cfg.Paths.BrewfileCLI)
}

// brewfiles are the CLI and GUI Brewfiles, once each when they are the same file.
func (r *repo) brewfiles() []string {
	cli, gui := r.brewfileFor(formula), r.brewfileFor(cask)
	if cli == gui {
		return []string{cli}
	}
	return []string{cli, gui}
}

// openRepo finds the repository and loads its dots.toml.
func openRepo() (*repo, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return nil, err
	}
	return &repo{root: root, cfg: cfg}, nil
}

// loadConfig reads dots.toml under root. A missing file yields the defaults.
func loadConfig(root string) (repoConfig, error) {
	cfg := defaultConfig()
	cfg.Version = 0
	md, err := toml.DecodeFile(filepath.Join(root, configFile), &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		cfg.Version = configVersion
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("cannot read %s: %w", configFile, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		for _, k := range keys {
			if s := k.String(); s == "paths.brewfile" || strings.HasPrefix(s, "brewfile") {
				return cfg, fmt.Errorf("%s: paths.brewfile and [brewfile] were replaced by paths.brewfile_cli (formulae) and paths.brewfile_gui (casks, mas apps)", configFile)
			}
		}
		return cfg, fmt.Errorf("%s: unknown keys: %v", configFile, keys)
	}
	if cfg.Version == 0 { // files written before versioning
		cfg.Version = 1
	}
	if cfg.Version > configVersion {
		return cfg, fmt.Errorf("%s is version %d, but this dots supports up to %d; upgrade dots",
			configFile, cfg.Version, configVersion)
	}
	cfg.cmdFrom = map[string]string{}
	for name := range cfg.Cmd {
		cfg.cmdFrom[name] = configFile
	}
	if err := loadIncludes(root, &cfg); err != nil {
		return cfg, err
	}
	for name, c := range cfg.Cmd {
		from := cfg.cmdFrom[name]
		if reservedNames[name] {
			return cfg, fmt.Errorf("%s: cmd.%s is a reserved name", from, name)
		}
		if c.Run == "" {
			return cfg, fmt.Errorf("%s: cmd.%s has no run", from, name)
		}
		if c.Dir != "" && c.Dir != "root" && c.Dir != "cwd" {
			return cfg, fmt.Errorf("%s: cmd.%s: dir must be root or cwd", from, name)
		}
	}
	return cfg, nil
}

// loadIncludes adds the [cmd.*] tables of the files listed under include.
// Included files may hold nothing else, and a name may be defined only once.
func loadIncludes(root string, cfg *repoConfig) error {
	for _, pattern := range cfg.Include {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return fmt.Errorf("%s: include %q: %w", configFile, pattern, err)
		}
		if len(matches) == 0 && !hasGlobMeta(pattern) {
			return fmt.Errorf("%s: include %q: file not found", configFile, pattern)
		}
		sort.Strings(matches)
		for _, path := range matches {
			rel, _ := filepath.Rel(root, path)
			var inc struct {
				Cmd map[string]customCmd `toml:"cmd"`
			}
			md, err := toml.DecodeFile(path, &inc)
			if err != nil {
				return fmt.Errorf("cannot read %s: %w", rel, err)
			}
			if keys := md.Undecoded(); len(keys) > 0 {
				return fmt.Errorf("%s: only [cmd.*] tables are allowed in included files: %v", rel, keys)
			}
			if cfg.Cmd == nil && len(inc.Cmd) > 0 {
				cfg.Cmd = map[string]customCmd{}
			}
			for name, c := range inc.Cmd {
				if prev, ok := cfg.cmdFrom[name]; ok {
					return fmt.Errorf("cmd.%s is defined in both %s and %s", name, prev, rel)
				}
				cfg.Cmd[name] = c
				cfg.cmdFrom[name] = rel
			}
		}
	}
	return nil
}

func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// userConfig is ~/.config/dots/config.toml, written by dots init.
type userConfig struct {
	Root string `toml:"root"` // the dotfiles repository
}

// userConfigPath honors $XDG_CONFIG_HOME and falls back to ~/.config
// (also on macOS, where os.UserConfigDir points to Application Support).
func userConfigPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "dots", "config.toml"), nil
}

func readUserConfig() (userConfig, error) {
	var uc userConfig
	path, err := userConfigPath()
	if err != nil {
		return uc, err
	}
	_, err = toml.DecodeFile(path, &uc)
	if errors.Is(err, fs.ErrNotExist) {
		return uc, nil
	}
	if err != nil {
		return uc, fmt.Errorf("cannot read %s: %w", tildePath(path), err)
	}
	return uc, nil
}

// repoRoot finds the dotfiles repository:
// $DOTS_ROOT, then root in the user config, then ~/dotfiles.
func repoRoot() (string, error) {
	dir := os.Getenv("DOTS_ROOT")
	if dir == "" {
		uc, err := readUserConfig()
		if err != nil {
			return "", err
		}
		dir = uc.Root
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "dotfiles")
	}
	dir, err := expandHome(dir)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("dotfiles repository not found: %s (run dots init, or set DOTS_ROOT)", tildePath(dir))
	}
	return dir, nil
}
