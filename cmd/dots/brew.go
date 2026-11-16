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

// section is the text of the Brewfile section heading that new entries go under.
func (k brewKind) section(cfg brewfileConfig) string {
	if k == cask {
		return cfg.CaskSection
	}
	return cfg.FormulaSection
}

func entryPattern(k brewKind, name string) *regexp.Regexp {
	return regexp.MustCompile(`^\s*` + string(k) + `\s+"` + regexp.QuoteMeta(name) + `"`)
}

func brewAdd(r *repo, k brewKind, names []string, comment string) error {
	brewfile := r.brewfile()
	lines, err := readLines(brewfile)
	if err != nil {
		return err
	}

	// Skip names already in the Brewfile; check that the rest exist in Homebrew.
	var targets []string
	for _, name := range names {
		if containsEntry(lines, k, name) {
			fmt.Printf("%s %q is already in the Brewfile\n", k, name)
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
		lines = insertIntoSection(lines, k.section(r.cfg.Brewfile), entry)
	}
	if err := writeLines(brewfile, lines); err != nil {
		return err
	}
	fmt.Printf("Added to the Brewfile: %s\n", strings.Join(targets, " "))
	return nil
}

func brewRemove(r *repo, k brewKind, names []string) error {
	brewfile := r.brewfile()
	lines, err := readLines(brewfile)
	if err != nil {
		return err
	}

	for _, name := range names {
		if !containsEntry(lines, k, name) {
			return fmt.Errorf("%s %q is not in the Brewfile", k, name)
		}
		re := entryPattern(k, name)
		kept := lines[:0:0]
		for _, l := range lines {
			if !re.MatchString(l) {
				kept = append(kept, l)
			}
		}
		lines = kept
	}
	if err := writeLines(brewfile, lines); err != nil {
		return err
	}
	fmt.Printf("Removed from the Brewfile: %s\n", strings.Join(names, " "))

	// Removing the entry leaves the package installed, so ask separately.
	if !confirm("Uninstall " + strings.Join(names, " ") + "?") {
		fmt.Println("Left installed")
		return nil
	}
	return run(r.root, "brew", append([]string{"uninstall"}, append(k.brewFlags(), names...)...)...)
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

// insertIntoSection adds entry after the last line of the section whose
// heading ("# --- ...") contains key. Without such a section (or with an
// empty key), the entry is appended to the end of the file.
func insertIntoSection(lines []string, key, entry string) []string {
	start := -1
	for i, l := range lines {
		if key != "" && strings.HasPrefix(l, "# --- ") && strings.Contains(l, key) {
			start = i
			break
		}
	}
	if start < 0 {
		return append(lines, entry)
	}

	// Insert before the blank lines that precede the next heading (or EOF).
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "# --- ") {
			end = i
			break
		}
	}
	at := end
	for at > start+1 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:at]...)
	out = append(out, entry)
	return append(out, lines[at:]...)
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
