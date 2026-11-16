package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestContainsEntry(t *testing.T) {
	lines := strings.Split("brew \"git\"\nbrew \"tree\"  # tree view\ncask \"ghostty\"\nmas \"Xcode\", id: 497799835", "\n")
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

func TestBrewfiles(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	r := &repo{root: root, cfg: cfg}
	writeFile(t, r.brewfileFor(formula), "brew \"git\"\n")
	writeFile(t, r.brewfileFor(cask), "cask \"ghostty\"\n")

	if got := r.brewfiles(); len(got) != 2 || got[0] != filepath.Join(root, "brew", "cli.Brewfile") {
		t.Errorf("brewfiles = %v", got)
	}
	// An entry is found in whichever file lists it, whatever its kind.
	if file, _ := findEntry(r, cask, "ghostty"); file != r.brewfileFor(cask) {
		t.Errorf("findEntry(cask ghostty) = %q", file)
	}
	if file, _ := findEntry(r, formula, "ghostty"); file != "" {
		t.Errorf("findEntry(brew ghostty) = %q, want none", file)
	}

	// The same file for both kinds is listed once.
	r.cfg.Paths.BrewfileGUI = r.cfg.Paths.BrewfileCLI
	if got := r.brewfiles(); len(got) != 1 {
		t.Errorf("brewfiles with one file = %v", got)
	}
}
