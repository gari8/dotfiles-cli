package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// configVersion is the newest dots.toml format this build understands.
const configVersion = 1

// configFile is the per-repository config at the root of the dotfiles repository.
const configFile = "dots.toml"

// repoConfig is the contents of dots.toml. Every field has a default, so an
// empty or missing dots.toml works.
type repoConfig struct {
	Version  int                  `toml:"version"`
	Paths    pathsConfig          `toml:"paths"`
	Brewfile brewfileConfig       `toml:"brewfile"`
	Links    map[string]string    `toml:"links"` // repo-relative source -> target (~ allowed)
	Cmd      map[string]customCmd `toml:"cmd"`
}

type pathsConfig struct {
	Brewfile   string `toml:"brewfile"`
	MiseConfig string `toml:"mise_config"`
	Install    string `toml:"install"`
}

type brewfileConfig struct {
	// Text that the section heading ("# --- ... ---") contains; new entries
	// are added at the end of the matching section.
	FormulaSection string `toml:"formula_section"`
	CaskSection    string `toml:"cask_section"`
}

func defaultConfig() repoConfig {
	return repoConfig{
		Version: configVersion,
		Paths: pathsConfig{
			Brewfile:   "Brewfile",
			MiseConfig: "config/mise/config.toml",
			Install:    "scripts/install.sh",
		},
		Brewfile: brewfileConfig{
			FormulaSection: "CLI",
			CaskSection:    "GUI",
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

func (r *repo) brewfile() string   { return r.path(r.cfg.Paths.Brewfile) }
func (r *repo) miseConfig() string { return r.path(r.cfg.Paths.MiseConfig) }

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
		return cfg, fmt.Errorf("%s: unknown keys: %v", configFile, keys)
	}
	if cfg.Version == 0 { // files written before versioning
		cfg.Version = 1
	}
	if cfg.Version > configVersion {
		return cfg, fmt.Errorf("%s is version %d, but this dots supports up to %d; upgrade dots",
			configFile, cfg.Version, configVersion)
	}
	for name, c := range cfg.Cmd {
		if reservedNames[name] {
			return cfg, fmt.Errorf("%s: cmd.%s is a reserved name", configFile, name)
		}
		if c.Run == "" {
			return cfg, fmt.Errorf("%s: cmd.%s has no run", configFile, name)
		}
		if c.Dir != "" && c.Dir != "root" && c.Dir != "cwd" {
			return cfg, fmt.Errorf("%s: cmd.%s: dir must be root or cwd", configFile, name)
		}
	}
	return cfg, nil
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
