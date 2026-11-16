package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScript(t *testing.T) {
	cases := map[string]string{
		"git push":            `git push "$@"`,
		`echo "first=$1"`:     `echo "first=$1"`,
		`git commit -m "$*"`:  `git commit -m "$*"`,
		"echo ${1:-x}":        "echo ${1:-x}",
		"a\nb":                "a\nb",
		"  git status -sb \n": `git status -sb "$@"`,
		`echo "$DOTS_ROOT"`:   `echo "$DOTS_ROOT" "$@"`,
	}
	for in, want := range cases {
		if got := script(in); got != want {
			t.Errorf("script(%q) = %q, want %q", in, got, want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()

	// Missing file: defaults.
	cfg, err := loadConfig(dir)
	if err != nil || cfg.Version != 1 || cfg.Paths.Brewfile != "Brewfile" {
		t.Fatalf("missing dots.toml: cfg=%+v err=%v", cfg, err)
	}

	// No version and partial paths: version 1, other defaults kept.
	writeFile(t, filepath.Join(dir, configFile), "[paths]\nbrewfile = \"brew/Brewfile\"\n")
	cfg, err = loadConfig(dir)
	if err != nil || cfg.Version != 1 || cfg.Paths.Brewfile != "brew/Brewfile" || cfg.Paths.MiseConfig != "config/mise/config.toml" {
		t.Fatalf("partial dots.toml: cfg=%+v err=%v", cfg, err)
	}

	errorCases := map[string]string{
		"version = 2\n":                       "upgrade dots",
		"[cmd.x]\nrun = \"a\"\ndesk = \"\"\n": "unknown keys",
		"[cmd.add]\nrun = \"a\"\n":            "reserved",
		"[cmd.x]\ndesc = \"a\"\n":             "no run",
		"[cmd.x]\nrun = \"a\"\ndir = \"x\"\n": "root or cwd",
		"[[[":                                 "cannot read",
	}
	for content, want := range errorCases {
		writeFile(t, filepath.Join(dir, configFile), content)
		if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want an error containing %q", content, err, want)
		}
	}
}

func TestRepoRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DOTS_ROOT", "")

	// Nothing set and no ~/dotfiles.
	if _, err := repoRoot(); err == nil || !strings.Contains(err.Error(), "dots init") {
		t.Fatalf("want a not-found error, got %v", err)
	}

	// ~/dotfiles is the fallback.
	os.Mkdir(filepath.Join(home, "dotfiles"), 0o755)
	if got, err := repoRoot(); err != nil || got != filepath.Join(home, "dotfiles") {
		t.Fatalf("fallback: got %q, %v", got, err)
	}

	// The user config wins over the fallback.
	other := filepath.Join(home, "src", "dots")
	os.MkdirAll(other, 0o755)
	writeFile(t, filepath.Join(home, ".config", "dots", "config.toml"), "root = \"~/src/dots\"\n")
	if got, err := repoRoot(); err != nil || got != other {
		t.Fatalf("user config: got %q, %v", got, err)
	}

	// $XDG_CONFIG_HOME moves the user config.
	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, err := repoRoot(); err != nil || got != filepath.Join(home, "dotfiles") {
		t.Fatalf("XDG_CONFIG_HOME without config: got %q, %v", got, err)
	}

	// $DOTS_ROOT wins over everything.
	t.Setenv("DOTS_ROOT", other)
	if got, err := repoRoot(); err != nil || got != other {
		t.Fatalf("DOTS_ROOT: got %q, %v", got, err)
	}
}

func TestInitAndLinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DOTS_ROOT", "")
	assumeYes = true
	defer func() { assumeYes = false }()

	root := filepath.Join(home, "dotfiles")
	writeFile(t, filepath.Join(root, "zsh", ".zshrc"), "# zshrc\n")
	writeFile(t, filepath.Join(root, "config", "mise", "config.toml"), "[tools]\n")
	writeFile(t, filepath.Join(home, ".zshrc"), "# old\n") // to be backed up

	// First run creates dots.toml from the template and registers the root.
	if err := initRepo(root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, configFile)); err != nil {
		t.Fatal("dots.toml was not created")
	}
	if got, _ := repoRoot(); got != root {
		t.Fatalf("root not registered: %q", got)
	}

	// Add links and run again.
	writeFile(t, filepath.Join(root, configFile), `[links]
"zsh/.zshrc" = "~/.zshrc"
"config/mise" = "~/.config/mise"
"missing" = "~/.missing"
`)
	if err := initRepo(root, false); err != nil {
		t.Fatal(err)
	}
	for src, to := range map[string]string{"zsh/.zshrc": ".zshrc", "config/mise": ".config/mise"} {
		dest, err := os.Readlink(filepath.Join(home, to))
		if err != nil || dest != filepath.Join(root, src) {
			t.Errorf("%s: link = %q, %v", to, dest, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".missing")); err == nil {
		t.Error("a link was created for a missing source")
	}
	backups, _ := filepath.Glob(filepath.Join(home, ".dotfiles_backup", "*", ".zshrc"))
	if len(backups) != 1 {
		t.Errorf("want 1 backup of .zshrc, got %v", backups)
	}

	// A third run changes nothing.
	if err := initRepo(root, false); err != nil {
		t.Fatal(err)
	}
	backups, _ = filepath.Glob(filepath.Join(home, ".dotfiles_backup", "*", ".zshrc*"))
	if len(backups) != 1 {
		t.Errorf("re-running init made more backups: %v", backups)
	}

	// Dry run changes nothing either.
	os.Remove(filepath.Join(home, ".zshrc"))
	if err := initRepo(root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err == nil {
		t.Error("dry run created a link")
	}
}
