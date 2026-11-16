package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// brewKind is the kind of a Brewfile entry (brew = formula, cask).
type brewKind string

const (
	formula brewKind = "brew"
	cask    brewKind = "cask"
)

func kindOf(isCask bool) brewKind {
	if isCask {
		return cask
	}
	return formula
}

// brewFlags are the flags that select the kind for brew install / info / uninstall.
func (k brewKind) brewFlags() []string {
	if k == cask {
		return []string{"--cask"}
	}
	return []string{"--formula"}
}

func entryPattern(k brewKind, name string) *regexp.Regexp {
	return regexp.MustCompile(`^\s*` + string(k) + `\s+"` + regexp.QuoteMeta(name) + `"`)
}

func brewAdd(r *repo, k brewKind, names []string, comment string) error {
	dest := r.brewfileFor(k)
	lines, err := readLines(dest)
	if err != nil {
		return err
	}

	// Skip names already in a Brewfile; check that the rest exist in Homebrew.
	var targets []string
	for _, name := range names {
		if file, _ := findEntry(r, k, name); file != "" {
			fmt.Printf("%s %q is already in %s\n", k, name, tildePath(file))
			continue
		}
		if !succeeds(r.root, "brew", append([]string{"info"}, append(k.brewFlags(), name)...)...) {
			return fmt.Errorf("%s %q not found in Homebrew", k, name)
		}
		targets = append(targets, name)
	}
	if len(targets) == 0 {
		return nil
	}

	if err := run(r.root, "brew", append([]string{"install"}, append(k.brewFlags(), targets...)...)...); err != nil {
		return err
	}

	for _, name := range targets {
		entry := fmt.Sprintf("%s %q", k, name)
		if comment != "" {
			entry += "  # " + comment
		}
		lines = append(lines, entry)
	}
	if err := writeLines(dest, lines); err != nil {
		return err
	}
	fmt.Printf("Added to %s: %s\n", tildePath(dest), strings.Join(targets, " "))
	return nil
}

func brewRemove(r *repo, k brewKind, names []string) error {
	for _, name := range names {
		file, lines := findEntry(r, k, name)
		if file == "" {
			return fmt.Errorf("%s %q is not in the Brewfiles", k, name)
		}
		re := entryPattern(k, name)
		kept := lines[:0:0]
		for _, l := range lines {
			if !re.MatchString(l) {
				kept = append(kept, l)
			}
		}
		if err := writeLines(file, kept); err != nil {
			return err
		}
		fmt.Printf("Removed from %s: %s\n", tildePath(file), name)
	}

	// Removing the entry leaves the package installed, so ask separately.
	if !confirm("Uninstall " + strings.Join(names, " ") + "?") {
		fmt.Println("Left installed")
		return nil
	}
	return run(r.root, "brew", append([]string{"uninstall"}, append(k.brewFlags(), names...)...)...)
}

// findEntry returns the Brewfile that lists name, and its lines.
func findEntry(r *repo, k brewKind, name string) (string, []string) {
	for _, file := range r.brewfiles() {
		lines, err := readLines(file)
		if err == nil && containsEntry(lines, k, name) {
			return file, lines
		}
	}
	return "", nil
}

func containsEntry(lines []string, k brewKind, name string) bool {
	re := entryPattern(k, name)
	for _, l := range lines {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}

func readLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n"), nil
}

func writeLines(path string, lines []string) error {
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
