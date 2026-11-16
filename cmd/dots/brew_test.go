package main

import (
	"strings"
	"testing"
)

const testBrewfile = `# Brewfile

# --- CLI tools ---
brew "git"
brew "tree"       # tree view

# --- GUI apps ---
cask "ghostty"    # terminal

# --- Mac App Store ---
mas "Xcode", id: 497799835`

func TestInsertIntoSection(t *testing.T) {
	cfg := defaultConfig().Brewfile
	lines := strings.Split(testBrewfile, "\n")
	lines = insertIntoSection(lines, formula.section(cfg), `brew "hello"`)
	lines = insertIntoSection(lines, cask.section(cfg), `cask "arc"  # browser`)
	got := strings.Join(lines, "\n")

	want := `# Brewfile

# --- CLI tools ---
brew "git"
brew "tree"       # tree view
brew "hello"

# --- GUI apps ---
cask "ghostty"    # terminal
cask "arc"  # browser

# --- Mac App Store ---
mas "Xcode", id: 497799835`
	if got != want {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestBrewfileTemplateSections(t *testing.T) {
	cfg := defaultConfig().Brewfile
	lines := strings.Split(strings.TrimRight(brewfileTemplate(cfg), "\n"), "\n")
	lines = insertIntoSection(lines, formula.section(cfg), `brew "tree"`)
	lines = insertIntoSection(lines, cask.section(cfg), `cask "arc"`)
	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "# --- CLI ---\nbrew \"tree\"\n") || !strings.HasSuffix(got, "# --- GUI ---\ncask \"arc\"") {
		t.Errorf("entries are not under their sections:\n%s", got)
	}
}

func TestInsertIntoSectionMissing(t *testing.T) {
	for _, key := range []string{"GUI", ""} {
		lines := insertIntoSection([]string{`brew "git"`}, key, `cask "arc"`)
		if got := strings.Join(lines, "\n"); got != "brew \"git\"\ncask \"arc\"" {
			t.Errorf("key %q: without a section the entry goes at the end: got %q", key, got)
		}
	}
}

func TestContainsEntry(t *testing.T) {
	lines := strings.Split(testBrewfile, "\n")
	cases := []struct {
		k    brewKind
		name string
		want bool
	}{
		{formula, "tree", true},
		{formula, "tre", false}, // no prefix match
		{cask, "tree", false},   // the kind must match
		{cask, "ghostty", true},
		{formula, "ghostty", false},
	}
	for _, c := range cases {
		if got := containsEntry(lines, c.k, c.name); got != c.want {
			t.Errorf("containsEntry(%s, %q) = %v, want %v", c.k, c.name, got, c.want)
		}
	}
}
