package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var t target
	cmd := &cobra.Command{
		Use:     "list [--mise|--brew]",
		Aliases: []string{"ls"},
		Short:   "Show managed tools and whether they are installed",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer w.Flush()
			if t.all() || t.mise {
				if err := listMise(w, r); err != nil {
					return err
				}
			}
			if t.all() || t.brew {
				if err := listBrew(w, r); err != nil {
					return err
				}
			}
			return nil
		},
	}
	addTargetFlags(cmd, &t, false, false)
	return cmd
}

// listItem is one row: name, whether it is installed, and a note.
type listItem struct {
	name, note string
	installed  bool
}

// printGroup prints a heading and its rows, indented by indent.
func printGroup(w *tabwriter.Writer, indent, title string, items []listItem) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "%s%s\n", indent, title)
	for _, it := range items {
		mark := "✓"
		if !it.installed {
			mark = "✗"
		}
		fmt.Fprintf(w, "%s  %s %s\t%s\n", indent, mark, it.name, it.note)
	}
}

func listMise(w *tabwriter.Writer, r *repo) error {
	if err := ready("mise", r.miseConfig()); err != nil {
		return err
	}
	out, err := output(r.root, "env", miseEnv(r), "mise", "ls", "--global", "--json")
	if err != nil {
		return err
	}
	var tools map[string][]struct {
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
	}
	if err := json.Unmarshal([]byte(out), &tools); err != nil {
		return fmt.Errorf("cannot parse mise ls output: %w", err)
	}

	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	var items []listItem
	for _, name := range names {
		for _, v := range tools[name] {
			items = append(items, listItem{name: name, note: v.Version, installed: v.Installed})
		}
	}
	if len(items) > 0 {
		printGroup(w, "", "mise", items)
		fmt.Fprintln(w)
	}
	return nil
}

// Brewfile lines: brew "name"  # comment / mas "Name", id: 123  # comment
var (
	brewLine = regexp.MustCompile(`^\s*(brew|cask)\s+"([^"]+)"[^#]*(?:#\s*(.*))?$`)
	masLine  = regexp.MustCompile(`^\s*mas\s+"([^"]+)",\s*id:\s*(\d+)[^#]*(?:#\s*(.*))?$`)
)

func listBrew(w *tabwriter.Writer, r *repo) error {
	if err := ready("brew", r.brewfile()); err != nil {
		return err
	}
	lines, err := readLines(r.brewfile())
	if err != nil {
		return err
	}
	formulae := installedSet(r.root, 0, "brew", "list", "--formula", "-1")
	casks := installedSet(r.root, 0, "brew", "list", "--cask", "-1")
	apps := installedSet(r.root, 0, "mas", "list") // the first column is the App Store id

	groups := map[string][]listItem{}
	for _, l := range lines {
		if m := brewLine.FindStringSubmatch(l); m != nil {
			set := formulae
			if m[1] == "cask" {
				set = casks
			}
			// brew list shows tapped names (owner/tap/name) as just name
			groups[m[1]] = append(groups[m[1]], listItem{name: m[2], note: m[3], installed: set[path.Base(m[2])]})
		} else if m := masLine.FindStringSubmatch(l); m != nil {
			groups["mas"] = append(groups["mas"], listItem{name: m[1], note: m[3], installed: apps[m[2]]})
		}
	}
	// Formulae, casks and mas apps are all installed from the Brewfile by
	// brew bundle, so they are grouped under brew.
	fmt.Fprintln(w, "brew")
	printGroup(w, "  ", "formula", groups["brew"])
	printGroup(w, "  ", "cask", groups["cask"])
	printGroup(w, "  ", "mas", groups["mas"])
	return nil
}

// ready checks that a manager (brew / mise) is installed and its config file
// exists; both are required.
func ready(tool, config string) error {
	if !exists(config) {
		return fmt.Errorf("not found: %s (dots init creates it)", tildePath(config))
	}
	return requireTool(tool)
}

// installedSet collects column col of each output line (empty if the command fails).
func installedSet(dir string, col int, name string, args ...string) map[string]bool {
	set := map[string]bool{}
	out, err := output(dir, name, args...)
	if err != nil {
		return set
	}
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > col {
			set[f[col]] = true
		}
	}
	return set
}
