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

Each finding says what to do: "dots upgrade" when a newer version of the
tool is out, "wait for ..." when the fix exists upstream but the tool (or
Homebrew) has not shipped it yet, nothing when there is no fix.
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
	pkg          string   // vulnerable package inside the tool (mise), or "" for the tool itself
	versions     []string // versions of pkg found (a library can be bundled several times)
	fix          string   // human-readable fix status
	fixed        bool     // a released fix exists
}

// What can be done about a finding.
const (
	actionUpgrade = iota // a newer version of the tool is out: dots upgrade
	actionWait           // fixed upstream, but the tool (or Homebrew) has not shipped it yet
	actionNone           // no fix yet
)

// action is what the user can do about v in tool t (kind is "brew" or "mise").
func (v scanVuln) action(t scanTool, kind string) (int, string) {
	switch {
	case !v.fixed:
		return actionNone, ""
	case t.newer != "":
		return actionUpgrade, "dots upgrade"
	case kind == "brew":
		return actionWait, "wait for Homebrew to update " + t.name
	default:
		return actionWait, "wait for a new " + t.name + " release"
	}
}

var severityRank = map[string]int{"CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3}

// tidyVulns merges findings that differ only in the bundled version and
// sorts them by severity.
func tidyVulns(vulns []scanVuln) []scanVuln {
	var out []scanVuln
	index := map[string]int{}
	for _, v := range vulns {
		key := v.id + "\x00" + v.pkg + "\x00" + v.fix
		if i, ok := index[key]; ok {
			out[i].versions = append(out[i].versions, v.versions...)
			continue
		}
		index[key] = len(out)
		out = append(out, v)
	}
	rank := func(s string) int {
		if r, ok := severityRank[strings.ToUpper(s)]; ok {
			return r
		}
		return len(severityRank)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := rank(out[i].severity), rank(out[j].severity); ri != rj {
			return ri < rj
		}
		if out[i].pkg != out[j].pkg {
			return out[i].pkg < out[j].pkg
		}
		return out[i].id < out[j].id
	})
	return out
}

// pkgLabel is the package and its versions, shortened when there are many.
func (v scanVuln) pkgLabel() string {
	if v.pkg == "" {
		return ""
	}
	switch n := len(v.versions); {
	case n == 0:
		return v.pkg
	case n <= 2:
		return v.pkg + " " + strings.Join(v.versions, ", ")
	default:
		return fmt.Sprintf("%s %s … %s (%d versions)", v.pkg, v.versions[0], v.versions[n-1], n)
	}
}

func printScan(groups []scanGroup) int {
	counts := [3]int{}
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
			state := "latest"
			if t.newer != "" {
				state = t.newer + " available"
			}
			vulns := tidyVulns(t.vulns)
			// When every finding calls for the same thing, say it once on the tool line.
			todos := map[string]bool{}
			for _, v := range vulns {
				_, todo := v.action(t, g.title)
				todos[todo] = true
			}
			header := fmt.Sprintf("  %s %s  (%s)", t.name, t.version, state)
			shared := len(todos) == 1
			if _, todo := vulns[0].action(t, g.title); shared && todo != "" {
				header += ": " + todo
			}
			fmt.Fprintln(w, header)
			for _, v := range vulns {
				a, todo := v.action(t, g.title)
				counts[a]++
				if shared {
					todo = ""
				}
				fmt.Fprintf(w, "    %s\t%s\t%s\t%s\t%s\n", strings.ToUpper(v.severity), v.id, v.pkgLabel(), v.fix, todo)
			}
		}
		for _, n := range g.notes {
			fmt.Fprintf(w, "  (%s)\n", n)
		}
	}
	w.Flush()
	total := counts[actionUpgrade] + counts[actionWait] + counts[actionNone]
	if total > 0 {
		fmt.Printf("\n%d vulnerabilities: %d fixed by dots upgrade, %d waiting for a new release of the tool, %d with no fix yet\n",
			total, counts[actionUpgrade], counts[actionWait], counts[actionNone])
		if counts[actionUpgrade] > 0 {
			fmt.Println("Run dots upgrade, then dots scan again.")
		}
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
			pkg:      m.Artifact.Name,
			versions: []string{m.Artifact.Version},
		}
		if key := sv.id + " " + sv.pkg + " " + m.Artifact.Version; seen[key] {
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
