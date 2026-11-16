package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"github.com/BurntSushi/toml"
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
	if err != nil || cfg.Version != 1 || cfg.Paths.BrewfileCLI != "brew/cli.Brewfile" {
		t.Fatalf("missing dots.toml: cfg=%+v err=%v", cfg, err)
	}

	// No version and partial paths: version 1, other defaults kept.
	writeFile(t, filepath.Join(dir, configFile), "[paths]\nbrewfile_gui = \"Brewfile\"\n")
	cfg, err = loadConfig(dir)
	if err != nil || cfg.Version != 1 || cfg.Paths.BrewfileGUI != "Brewfile" || cfg.Paths.BrewfileCLI != "brew/cli.Brewfile" || cfg.Paths.MiseConfig != "config/mise/config.toml" {
		t.Fatalf("partial dots.toml: cfg=%+v err=%v", cfg, err)
	}

	errorCases := map[string]string{
		"version = 2\n":                           "upgrade dots",
		"[cmd.x]\nrun = \"a\"\ndesk = \"\"\n":     "unknown keys",
		"[cmd.add]\nrun = \"a\"\n":                "reserved",
		"[cmd.x]\ndesc = \"a\"\n":                 "no run",
		"[cmd.x]\nrun = \"a\"\ndir = \"x\"\n":     "root or cwd",
		"[[[":                                     "cannot read",
		"[paths]\nbrewfile = \"B\"\n":             "brewfile_cli",
		"[brewfile]\nformula_section = \"CLI\"\n": "brewfile_cli",
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
	if err := initRepo(root, initPaths{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, configFile)); err != nil {
		t.Fatal("dots.toml was not created")
	}
	if got, _ := repoRoot(); got != root {
		t.Fatalf("root not registered: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "brew", "gui.Brewfile")); err != nil {
		t.Error("the GUI Brewfile was not created")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "config", "mise", "config.toml")); string(b) != "[tools]\n" {
		t.Errorf("an existing mise config was overwritten: %q", b)
	}

	// Add links and run again.
	writeFile(t, filepath.Join(root, configFile), `[links]
"zsh/.zshrc" = "~/.zshrc"
"config/mise" = "~/.config/mise"
"missing" = "~/.missing"
`)
	if err := initRepo(root, initPaths{}, false); err != nil {
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
	if err := initRepo(root, initPaths{}, false); err != nil {
		t.Fatal(err)
	}
	backups, _ = filepath.Glob(filepath.Join(home, ".dotfiles_backup", "*", ".zshrc*"))
	if len(backups) != 1 {
		t.Errorf("re-running init made more backups: %v", backups)
	}

	// Dry run changes nothing either.
	os.Remove(filepath.Join(home, ".zshrc"))
	if err := initRepo(root, initPaths{}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err == nil {
		t.Error("dry run created a link")
	}
}

func TestBrewFix(t *testing.T) {
	cases := []struct {
		in    []string
		want  string
		fixed bool
	}{
		{nil, "no fix yet", false},
		{[]string{"1.2.3"}, "fixed in 1.2.3", true},
		{[]string{"0a1b2c3d4e5f60718293a4b5c6d7e8f901234567"}, "patched upstream, not released yet", false},
		{[]string{"0a1b2c3", "2.0.1"}, "fixed in 2.0.1", true},
	}
	for _, c := range cases {
		if got, fixed := brewFix(c.in); got != c.want || fixed != c.fixed {
			t.Errorf("brewFix(%v) = %q, %v; want %q, %v", c.in, got, fixed, c.want, c.fixed)
		}
	}
}

func TestSetTOMLKey(t *testing.T) {
	cases := []struct{ in, want string }{
		// replace the existing key, keep comments and other keys
		{"# top\n[paths]\nbrewfile = \"Brewfile\"  # old\nmise_config = \"m.toml\"\n\n[links]\n",
			"# top\n[paths]\nbrewfile = \"brew/Brewfile\"\nmise_config = \"m.toml\"\n\n[links]\n"},
		// add the key to an existing section
		{"[paths]\nmise_config = \"m.toml\"\n", "[paths]\nbrewfile = \"brew/Brewfile\"\nmise_config = \"m.toml\"\n"},
		// a key with the same name in another section is not touched
		{"[other]\nbrewfile = \"x\"\n", "[other]\nbrewfile = \"x\"\n\n[paths]\nbrewfile = \"brew/Brewfile\"\n"},
	}
	for _, c := range cases {
		if got := setTOMLKey(c.in, "paths", "brewfile", "brew/Brewfile"); got != c.want {
			t.Errorf("setTOMLKey(%q) =\n%q\nwant\n%q", c.in, got, c.want)
		}
	}
}

func TestInitPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DOTS_ROOT", "")
	root := filepath.Join(home, "dotfiles")
	writeFile(t, filepath.Join(root, "mise.toml"), "[tools]\njq = \"latest\"\n")
	writeFile(t, filepath.Join(home, "Brewfile"), "cask \"ghostty\"\n") // outside the repository

	t.Chdir(root)
	if err := initRepo(root, initPaths{brewfileGUI: "~/Brewfile", miseConfig: "mise.toml"}, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Paths.MiseConfig != "mise.toml" || cfg.Paths.BrewfileGUI != filepath.Join(home, "Brewfile") {
		t.Errorf("paths = %+v", cfg.Paths)
	}
	// The existing files are used, not replaced by templates.
	if _, err := os.Stat(filepath.Join(root, "config", "mise", "config.toml")); err == nil {
		t.Error("a default mise config was created although --mise-config was given")
	}
	if _, err := os.Stat(filepath.Join(root, "brew", "gui.Brewfile")); err == nil {
		t.Error("a default GUI Brewfile was created although --brewfile-gui was given")
	}
}

func TestParseRemote(t *testing.T) {
	cases := []struct {
		in   string
		want remoteSpec
	}{
		{"git@github.com:gari8/dotfiles.git//Brewfile", remoteSpec{"git@github.com:gari8/dotfiles.git", "Brewfile", ""}},
		{"git@github.com:gari8/dotfiles.git", remoteSpec{"git@github.com:gari8/dotfiles.git", "dots.toml", ""}},
		{"https://github.com/gari8/dotfiles.git//config/mise/config.toml?ref=dev", remoteSpec{"https://github.com/gari8/dotfiles.git", "config/mise/config.toml", "dev"}},
		{"https://github.com/gari8/dotfiles", remoteSpec{"https://github.com/gari8/dotfiles", "dots.toml", ""}},
		{"file:///tmp/repo//a/b", remoteSpec{"file:///tmp/repo", "a/b", ""}},
	}
	for _, c := range cases {
		if got := parseRemote(c.in, "dots.toml"); got != c.want {
			t.Errorf("parseRemote(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
	for _, s := range []string{"./Brewfile", "Brewfile", "~/x", "/abs/path"} {
		if isRemote(s) {
			t.Errorf("isRemote(%q) = true", s)
		}
	}
}

func TestFormatAndRemoveTable(t *testing.T) {
	c := customCmd{Desc: "Push", Confirm: true, Run: "st=$(git status)\n[ -n \"$st\" ] && echo dirty\ngit push\n"}
	block := formatCmd("push", c)
	doc := "version = 1\n\n[cmd.a]\nrun = \"a\"\n\n" + block + "\n\n[cmd.b]\nrun = \"b\"\n"

	// The formatted block parses back to the same command.
	var parsed struct {
		Cmd map[string]customCmd `toml:"cmd"`
	}
	if _, err := toml.Decode(doc, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Cmd["push"] != c {
		t.Errorf("round trip: got %+v, want %+v", parsed.Cmd["push"], c)
	}

	// A line starting with [ inside the script is not taken for a table header.
	got := removeTable(doc, "cmd.push")
	if strings.Contains(got, "git push") || !strings.Contains(got, "[cmd.a]") || !strings.Contains(got, "[cmd.b]") {
		t.Errorf("removeTable:\n%s", got)
	}
	if removeTable(doc, "cmd.none") != doc {
		t.Error("removing a missing table changed the document")
	}
}

func TestInclude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, configFile), "include = [\"cmds/*.toml\"]\n\n[cmd.status]\nrun = \"git status\"\n")
	writeFile(t, filepath.Join(dir, "cmds", "git.toml"), "[cmd.push]\nrun = \"git push\"\n")
	writeFile(t, filepath.Join(dir, "cmds", "work.toml"), "[cmd.deploy]\nrun = \"make deploy\"\n")

	cfg, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Cmd) != 3 || cfg.cmdFrom["push"] != filepath.Join("cmds", "git.toml") || cfg.cmdFrom["status"] != configFile {
		t.Fatalf("cmd = %v, from = %v", cfg.Cmd, cfg.cmdFrom)
	}

	errorCases := map[string]string{
		"[cmd.status]\nrun = \"x\"\n": "defined in both",
		"[links]\n\"a\" = \"~/a\"\n":  "only [cmd.*]",
		"[cmd.add]\nrun = \"x\"\n":    "reserved",
		"[cmd.x]\ndesc = \"x\"\n":     "cmds/bad.toml: cmd.x has no run",
	}
	for content, want := range errorCases {
		writeFile(t, filepath.Join(dir, "cmds", "bad.toml"), content)
		if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want an error containing %q", content, err, want)
		}
	}
	os.Remove(filepath.Join(dir, "cmds", "bad.toml"))

	// A literal path that does not exist is an error; an empty glob is not.
	writeFile(t, filepath.Join(dir, configFile), "include = [\"none/*.toml\", \"missing.toml\"]\n")
	if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), "missing.toml") {
		t.Errorf("missing include: got %v", err)
	}
}

func TestInitGitInit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DOTS_ROOT", "")
	root := filepath.Join(home, "dotfiles")
	os.MkdirAll(root, 0o755) // made by hand, not a git repository

	if err := initRepo(root, initPaths{}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		t.Fatal("dry run ran git init")
	}
	if err := initRepo(root, initPaths{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatal("git init was not run for an existing directory")
	}
}

func TestInitFromReview(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DOTS_ROOT", "")
	root := filepath.Join(home, "dotfiles")
	writeFile(t, filepath.Join(root, "zsh", ".zshrc"), "# theirs\n")
	writeFile(t, filepath.Join(root, configFile), "[links]\n\"zsh/.zshrc\" = \"~/.zshrc\"\n")
	writeFile(t, filepath.Join(home, ".zshrc"), "# mine\n")
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)

	// Answer no: nothing in the home directory changes.
	oldStdin := stdin
	defer func() { stdin = oldStdin }()
	stdin = bufio.NewReader(strings.NewReader("n\n"))
	if err := initRepo(root, initPaths{cloned: true}, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(b) != "# mine\n" {
		t.Fatalf("~/.zshrc was replaced although the answer was no: %q", b)
	}

	// Answer yes: linked.
	stdin = bufio.NewReader(strings.NewReader("y\n"))
	if err := initRepo(root, initPaths{cloned: true}, false); err != nil {
		t.Fatal(err)
	}
	if dest, _ := os.Readlink(filepath.Join(home, ".zshrc")); dest != filepath.Join(root, "zsh", ".zshrc") {
		t.Fatalf("~/.zshrc not linked: %q", dest)
	}
}

func TestMissingShellSetup(t *testing.T) {
	home := "/home/u"
	files := map[string]string{}
	read := func(p string) string { return files[p] }
	brew := "/opt/homebrew/bin/brew"
	sysPath := []string{"/usr/bin", "/bin"}

	// Nothing set up: all three lines, in the login file and the rc file.
	got := missingShellSetup(home, "zsh", brew, sysPath, read)
	want := []shellLine{
		{"~/.zprofile", `export PATH="$HOME/.local/bin:$PATH"`},
		{"~/.zprofile", `eval "$(/opt/homebrew/bin/brew shellenv)"`},
		{"~/.zshrc", `eval "$(mise activate zsh)"`},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %v, want %v", i, got[i], want[i])
		}
	}

	// Lines anywhere in the startup files count, and dirs already on PATH need nothing.
	files["/home/u/.zshrc"] = "eval \"$(/opt/homebrew/bin/brew shellenv)\"\neval \"$(mise activate zsh)\"\n"
	got = missingShellSetup(home, "zsh", brew, append(sysPath, "/home/u/.local/bin"), read)
	if len(got) != 0 {
		t.Errorf("set up: got %v", got)
	}

	// Intel Homebrew in /usr/local/bin is on PATH already.
	got = missingShellSetup(home, "bash", "/usr/local/bin/brew", []string{"/usr/local/bin", "/home/u/.local/bin"}, func(string) string { return "" })
	if len(got) != 1 || got[0].line != `eval "$(mise activate bash)"` || got[0].file != "~/.bashrc" {
		t.Errorf("bash: got %v", got)
	}

	if got := missingShellSetup(home, "tcsh", brew, sysPath, read); got != nil {
		t.Errorf("unknown shell: got %v", got)
	}
}

func TestTidyAndAction(t *testing.T) {
	vulns := []scanVuln{
		{id: "B", severity: "High", pkg: "golang.org/x/mod", versions: []string{"v0.14.0"}, fix: "fixed in 0.40.0", fixed: true},
		{id: "A", severity: "Unknown", pkg: "stdlib", versions: []string{"go1.27.3"}, fix: "fixed in 1.27.4", fixed: true},
		{id: "B", severity: "High", pkg: "golang.org/x/mod", versions: []string{"v0.19.0"}, fix: "fixed in 0.40.0", fixed: true},
		{id: "B", severity: "High", pkg: "golang.org/x/mod", versions: []string{"v0.20.0"}, fix: "fixed in 0.40.0", fixed: true},
		{id: "C", severity: "Critical", pkg: "golang.org/x/net", versions: []string{"v0.57.0"}, fix: "no fix yet"},
	}
	got := tidyVulns(vulns)
	if len(got) != 3 {
		t.Fatalf("merged = %+v", got)
	}
	if got[0].id != "C" || got[1].id != "B" || got[2].id != "A" {
		t.Errorf("not sorted by severity: %v %v %v", got[0].id, got[1].id, got[2].id)
	}
	if l := got[1].pkgLabel(); l != "golang.org/x/mod v0.14.0 … v0.20.0 (3 versions)" {
		t.Errorf("label = %q", l)
	}

	latest := scanTool{name: "kubectl"}
	outdated := scanTool{name: "gh", newer: "2.104.0"}
	cases := []struct {
		v      scanVuln
		t      scanTool
		kind   string
		action int
		todo   string
	}{
		{got[1], outdated, "mise", actionUpgrade, "dots upgrade"},
		{got[1], latest, "mise", actionWait, "wait for a new kubectl release"},
		{scanVuln{fixed: true}, scanTool{name: "openssl@4"}, "brew", actionWait, "wait for Homebrew to update openssl@4"},
		{got[0], outdated, "mise", actionNone, ""},
	}
	for _, c := range cases {
		if a, todo := c.v.action(c.t, c.kind); a != c.action || todo != c.todo {
			t.Errorf("action(%s, %s) = %d %q, want %d %q", c.v.id, c.t.name, a, todo, c.action, c.todo)
		}
	}
}

func TestVersions(t *testing.T) {
	cases := []struct {
		current, latest string
		newer           bool
	}{
		{"0.1.6", "0.1.7", true},
		{"0.1.7", "0.1.7", false},
		{"0.2.0", "0.1.9", false},
		{"0.9.0", "0.10.0", true},
		{"1.0.0-rc.1", "1.0.0", false}, // suffixes are ignored
		{"dev", "0.1.7", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.current, c.latest); got != c.newer {
			t.Errorf("newerVersion(%q, %q) = %v", c.current, c.latest, got)
		}
	}
}

func TestReleaseArchive(t *testing.T) {
	// A tar.gz like goreleaser's, with dots and a README.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"README.md": "readme", "dots": "binary"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	data := buf.Bytes()

	sum := sha256.Sum256(data)
	sums := hex.EncodeToString(sum[:]) + "  dotfiles-cli_0.1.7_darwin_arm64.tar.gz\n"
	if err := checkSum(data, sums, "dotfiles-cli_0.1.7_darwin_arm64.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if err := checkSum(append(data, 0), sums, "dotfiles-cli_0.1.7_darwin_arm64.tar.gz"); err == nil {
		t.Error("a changed archive passed the checksum")
	}
	if err := checkSum(data, sums, "other.tar.gz"); err == nil {
		t.Error("a missing checksum line passed")
	}
	bin, err := extractDots(data)
	if err != nil || string(bin) != "binary" {
		t.Errorf("extractDots = %q, %v", bin, err)
	}
}

func TestShortVersion(t *testing.T) {
	cases := map[string]string{
		"v0.55.1-0.20260731170536-c1d18010be90":     "v0.55.1 (dev 2026-07-31)",
		"0.58.1-0.20261009181718-2f1f834330b2":      "0.58.1 (dev 2026-10-09)",
		"v0.0.0-20260101120000-abcdefabcdef":        "v0.0.0 (dev 2026-01-01)",
		"v1.2.3-rc.1.0.20260202030405-0123456789ab": "v1.2.3 (dev 2026-02-02)",
		"v0.57.0":  "v0.57.0",
		"go1.27.3": "go1.27.3",
	}
	for in, want := range cases {
		if got := shortVersion(in); got != want {
			t.Errorf("shortVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
