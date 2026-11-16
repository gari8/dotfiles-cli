package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// grype scans mise tools; it is run through mise so it does not have to be
// added to the mise config.
const grype = "grype@latest"

func newScanCmd() *cobra.Command {
	var t target
	cmd := &cobra.Command{
		Use:   "scan [--mise|--brew]",
		Short: "Check managed tools for known vulnerabilities and available fixes",
		Long: `Check managed tools for known vulnerabilities and whether a fix is out.

  brew: Brewfile formulae and their dependencies, with brew vulns (OSV.dev).
        Casks and mas apps are not covered.
  mise: installed tool versions, with grype (run via mise exec ` + grype + `).
        Findings are usually in libraries bundled into a tool.

Exits 1 when anything is found.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			doBrew, doMise := t.all() || t.brew, t.all() || t.mise
			var groups []scanGroup
			if doBrew {
				if err := ready("brew", r.brewfiles()...); err != nil {
					return err
				}
				g, err := scanBrew(r)
				if err != nil {
					return err
				}
				groups = append(groups, g)
			}
			if doMise {
				if err := ready("mise", r.miseConfig()); err != nil {
					return err
				}
				g, err := scanMise(r)
				if err != nil {
					return err
				}
				groups = append(groups, g)
			}
			if printScan(groups) > 0 {
				return exitCodeError(1)
			}
			return nil
		},
	}
	addTargetFlags(cmd, &t, false, false)
	return cmd
}

// scanGroup is the result for one manager.
type scanGroup struct {
	title string
	tools []scanTool
	notes []string // coverage notes shown under the group
}

// scanTool is one installed tool with its findings.
type scanTool struct {
	name, version string
	newer         string // newer version of the tool itself, if any
	vulns         []scanVuln
}

type scanVuln struct {
	id, severity string
	pkg          string // vulnerable package inside the tool (mise), or ""
	fix          string // human-readable fix status
	fixed        bool   // a released fix exists
}

func printScan(groups []scanGroup) int {
	total, fixable := 0, 0
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, g := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, g.title)
		if len(g.tools) == 0 {
			fmt.Fprintln(w, "  no known vulnerabilities")
		}
		for _, t := range g.tools {
			line := fmt.Sprintf("  %s %s", t.name, t.version)
			if t.newer != "" {
				line += fmt.Sprintf("  (%s available: dots upgrade)", t.newer)
			}
			fmt.Fprintln(w, line)
			for _, v := range t.vulns {
				fmt.Fprintf(w, "    %s\t%s\t%s\t%s\n", strings.ToUpper(v.severity), v.id, v.pkg, v.fix)
				total++
				if v.fixed {
					fixable++
				}
			}
		}
		for _, n := range g.notes {
			fmt.Fprintf(w, "  (%s)\n", n)
		}
	}
	w.Flush()
	if total > 0 {
		fmt.Printf("\n%d vulnerabilities, %d with a released fix\n", total, fixable)
	}
	return total
}

// commitSHA matches a fix that is only a commit, not a release.
var commitSHA = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func scanBrew(r *repo) (scanGroup, error) {
	g := scanGroup{title: "brew"}
	newer := map[string]string{}
	if items, err := brewOutdated(r); err == nil {
		for _, it := range items {
			newer[it.name] = it.latest
		}
	}
	seen := map[string]bool{} // a dependency shared by both Brewfiles is reported once
	skipped := 0
	for _, file := range r.brewfiles() {
		n, err := scanBrewfile(r, file, newer, seen, &g)
		if err != nil {
			return g, err
		}
		skipped += n
	}
	if skipped > 0 {
		g.notes = append(g.notes, fmt.Sprintf("%d formulae could not be checked: brew vulns --list-skipped", skipped))
	}
	g.notes = append(g.notes, "casks and mas apps are not checked")
	return g, nil
}

// scanBrewfile adds the findings for one Brewfile to g and returns how many
// formulae brew vulns skipped.
func scanBrewfile(r *repo, file string, newer map[string]string, seen map[string]bool, g *scanGroup) (int, error) {
	// brew vulns exits 1 when it finds something, so read stdout either way.
	// Vulnerabilities that Homebrew's own patches resolve are under "patched"
	// and are not reported.
	cmd := exec.Command("brew", "vulns", "--brewfile="+file, "--deps", "--json")
	cmd.Dir = r.root
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	out := strings.TrimSpace(string(raw))
	if err != nil && out == "" {
		return 0, fmt.Errorf("brew vulns: %w", err)
	}
	var res struct {
		Findings []struct {
			Formula         string `json:"formula"`
			Version         string `json:"version"`
			Vulnerabilities []struct {
				ID            string   `json:"id"`
				Severity      string   `json:"severity"`
				FixedVersions []string `json:"fixed_versions"`
			} `json:"vulnerabilities"`
		} `json:"findings"`
		Skipped []any `json:"skipped_formulae"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return 0, fmt.Errorf("cannot parse brew vulns output: %w", err)
	}
	for _, f := range res.Findings {
		if seen[f.Formula] {
			continue
		}
		seen[f.Formula] = true
		t := scanTool{name: f.Formula, version: f.Version, newer: newer[f.Formula]}
		for _, v := range f.Vulnerabilities {
			sv := scanVuln{id: v.ID, severity: orUnknown(v.Severity)}
			sv.fix, sv.fixed = brewFix(v.FixedVersions)
			t.vulns = append(t.vulns, sv)
		}
		g.tools = append(g.tools, t)
	}
	return len(res.Skipped), nil
}

// brewFix describes fixed_versions: released versions, commits only, or nothing.
func brewFix(fixed []string) (string, bool) {
	var releases, commits []string
	for _, v := range fixed {
		if commitSHA.MatchString(v) {
			commits = append(commits, v)
		} else {
			releases = append(releases, v)
		}
	}
	switch {
	case len(releases) > 0:
		return "fixed in " + strings.Join(releases, ", "), true
	case len(commits) > 0:
		return "patched upstream, not released yet", false
	default:
		return "no fix yet", false
	}
}

func scanMise(r *repo) (scanGroup, error) {
	g := scanGroup{title: "mise"}
	out, err := output(r.root, "env", miseEnv(r), "mise", "ls", "--global", "--json")
	if err != nil {
		return g, err
	}
	var installed map[string][]struct {
		Version     string `json:"version"`
		InstallPath string `json:"install_path"`
		Installed   bool   `json:"installed"`
	}
	if err := json.Unmarshal([]byte(out), &installed); err != nil {
		return g, fmt.Errorf("cannot parse mise ls output: %w", err)
	}

	newer := map[string]string{}
	if items, err := miseOutdated(r); err == nil {
		for _, it := range items {
			if it.latest != it.current {
				newer[it.name] = it.latest
			}
		}
	}

	names := make([]string, 0, len(installed))
	for name := range installed {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(os.Stderr, "Scanning %d mise tools with grype...\n", len(names))
	for _, name := range names {
		for _, v := range installed[name] {
			if !v.Installed || v.InstallPath == "" {
				continue
			}
			vulns, err := grypeScan(r, v.InstallPath)
			if err != nil {
				return g, fmt.Errorf("%s: %w", name, err)
			}
			if len(vulns) > 0 {
				g.tools = append(g.tools, scanTool{name: name, version: v.Version, newer: newer[name], vulns: vulns})
			}
		}
	}
	return g, nil
}

// grypeScan scans one install directory and returns its findings.
func grypeScan(r *repo, dir string) ([]scanVuln, error) {
	out, err := output(r.root, "mise", "exec", grype, "--", "grype", "dir:"+dir, "-o", "json", "-q")
	if err != nil {
		return nil, err
	}
	var res struct {
		Matches []struct {
			Vulnerability struct {
				ID       string `json:"id"`
				Severity string `json:"severity"`
				Fix      struct {
					State    string   `json:"state"`
					Versions []string `json:"versions"`
				} `json:"fix"`
			} `json:"vulnerability"`
			Artifact struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"artifact"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, fmt.Errorf("cannot parse grype output: %w", err)
	}
	var vulns []scanVuln
	seen := map[string]bool{} // a library bundled several times is reported once
	for _, m := range res.Matches {
		v := m.Vulnerability
		sv := scanVuln{
			id:       v.ID,
			severity: orUnknown(v.Severity),
			pkg:      m.Artifact.Name + " " + m.Artifact.Version,
		}
		if key := sv.id + " " + sv.pkg; seen[key] {
			continue
		} else {
			seen[key] = true
		}
		if v.Fix.State == "fixed" && len(v.Fix.Versions) > 0 {
			sv.fix, sv.fixed = "fixed in "+strings.Join(v.Fix.Versions, ", "), true
		} else {
			sv.fix = "no fix yet"
		}
		vulns = append(vulns, sv)
	}
	return vulns, nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
