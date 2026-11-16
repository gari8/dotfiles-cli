package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newUpgradeCmd() *cobra.Command {
	var dryRun bool
	var exclude []string
	var t target
	cmd := &cobra.Command{
		Use:   "upgrade [--mise|--brew]",
		Short: "Show available upgrades and apply them after confirmation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			doBrew, doMise := t.all() || t.brew, t.all() || t.mise
			if doBrew {
				if err := ready("brew", r.brewfile()); err != nil {
					return err
				}
			}
			if doMise {
				if err := ready("mise", r.miseConfig()); err != nil {
					return err
				}
			}
			return upgrade(r, doBrew, doMise, dryRun, exclude)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "only show what would be upgraded")
	cmd.Flags().StringSliceVarP(&exclude, "exclude", "x", nil, "tools to leave as they are (comma separated)")
	addTargetFlags(cmd, &t, false, false)
	return cmd
}

// outdatedItem is a tool with a newer version available.
type outdatedItem struct {
	name, current, latest string
}

func upgrade(r *repo, doBrew, doMise, dryRun bool, exclude []string) error {
	var brewItems, miseItems []outdatedItem
	var err error
	if doBrew {
		// Refresh formula and cask definitions before comparing versions.
		if err := run(r.root, "brew", "update", "--quiet"); err != nil {
			return err
		}
		if brewItems, err = brewOutdated(r); err != nil {
			return err
		}
	}
	if doMise {
		if miseItems, err = miseOutdated(r); err != nil {
			return err
		}
	}
	brewItems = without(brewItems, exclude)
	miseItems = without(miseItems, exclude)

	if len(brewItems) == 0 && len(miseItems) == 0 {
		fmt.Println("Everything is up to date")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	printOutdated(w, "brew", brewItems)
	printOutdated(w, "mise", miseItems)
	w.Flush()

	if dryRun {
		return nil
	}
	if !confirm("Upgrade?") {
		return canceled("Upgrade")
	}
	// Name the tools explicitly so only what was shown gets upgraded.
	if len(brewItems) > 0 {
		if err := run(r.root, "brew", append([]string{"upgrade"}, names(brewItems)...)...); err != nil {
			return err
		}
	}
	if len(miseItems) > 0 {
		args := append([]string{miseEnv(r), "mise", "upgrade", "--global"}, names(miseItems)...)
		if err := run(r.root, "env", args...); err != nil {
			return err
		}
	}
	return nil
}

func printOutdated(w *tabwriter.Writer, title string, items []outdatedItem) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(w, title)
	for _, it := range items {
		if it.current == it.latest {
			// Channels such as rust "stable" keep the same name across builds.
			fmt.Fprintf(w, "  %s\t%s (new build)\n", it.name, it.current)
		} else {
			fmt.Fprintf(w, "  %s\t%s → %s\n", it.name, it.current, it.latest)
		}
	}
}

func brewOutdated(r *repo) ([]outdatedItem, error) {
	out, err := output(r.root, "brew", "outdated", "--json=v2")
	if err != nil {
		return nil, err
	}
	type pkg struct {
		Name      string   `json:"name"`
		Installed []string `json:"installed_versions"`
		Current   string   `json:"current_version"`
		Pinned    bool     `json:"pinned"`
	}
	var res struct {
		Formulae []pkg `json:"formulae"`
		Casks    []pkg `json:"casks"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, fmt.Errorf("cannot parse brew outdated output: %w", err)
	}
	var items []outdatedItem
	for _, p := range append(res.Formulae, res.Casks...) {
		if p.Pinned { // brew upgrade skips pinned packages
			continue
		}
		items = append(items, outdatedItem{p.Name, strings.Join(p.Installed, ", "), p.Current})
	}
	return items, nil
}

func miseOutdated(r *repo) ([]outdatedItem, error) {
	out, err := output(r.root, "env", miseEnv(r), "mise", "outdated", "--global", "--json")
	if err != nil {
		return nil, err
	}
	var res map[string]struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, fmt.Errorf("cannot parse mise outdated output: %w", err)
	}
	var items []outdatedItem
	for name, t := range res {
		items = append(items, outdatedItem{name, t.Current, t.Latest})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })
	return items, nil
}

// without drops the items whose name is in exclude.
func without(items []outdatedItem, exclude []string) []outdatedItem {
	skip := map[string]bool{}
	for _, name := range exclude {
		skip[name] = true
	}
	var kept []outdatedItem
	for _, it := range items {
		if !skip[it.name] {
			kept = append(kept, it)
		}
	}
	return kept
}

func names(items []outdatedItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.name
	}
	return out
}
