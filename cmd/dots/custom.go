package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// customCmd is one [cmd.<name>] table in dots.toml.
type customCmd struct {
	Run     string `toml:"run"`     // shell command (sh -c); see script for arguments
	Desc    string `toml:"desc"`    // shown by dots cmd list
	Dir     string `toml:"dir"`     // working directory: root (default) or cwd
	Confirm bool   `toml:"confirm"` // ask before running
}

// reservedNames are kept for dots cmd's own subcommands and cannot be used
// as custom command names.
var reservedNames = map[string]bool{
	"list": true, "ls": true,
	"add": true, "rm": true, "remove": true,
}

// exitCodeError passes a custom command's exit status through as dots's own.
type exitCodeError int

func (e exitCodeError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func newCmdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cmd [<name> [args]...]",
		Short: "Run a custom command from dots.toml (dots cmd list to see them)",
		// Pass every argument, flags included, to the custom command.
		DisableFlagParsing: true,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveDefault
			}
			completions := []string{"list\tlist custom commands"}
			if r, err := openRepo(); err == nil {
				for name, c := range r.cfg.Cmd {
					completions = append(completions, name+"\t"+c.describe())
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			args = stripYes(args)
			r, err := openRepo()
			if err != nil {
				return err
			}
			if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "list" || args[0] == "ls" {
				listCustom(r.cfg.Cmd)
				return nil
			}
			if reservedNames[args[0]] {
				return fmt.Errorf("dots cmd %s is reserved but not available yet", args[0])
			}
			c, ok := r.cfg.Cmd[args[0]]
			if !ok {
				return fmt.Errorf("no custom command %q (dots cmd list to see them)", args[0])
			}
			return runCustom(r.root, args[0], c, args[1:])
		},
	}
}

// stripYes removes leading -y / --yes and applies them; flag parsing is
// disabled for dots cmd, so dots cmd -y push needs this.
func stripYes(args []string) []string {
	for len(args) > 0 && (args[0] == "-y" || args[0] == "--yes") {
		assumeYes = true
		args = args[1:]
	}
	return args
}

func (c customCmd) describe() string {
	if c.Desc != "" {
		return c.Desc
	}
	return c.Run
}

func listCustom(cmds map[string]customCmd) {
	if len(cmds) == 0 {
		fmt.Printf("No custom commands (add [cmd.<name>] tables to %s)\n", configFile)
		return
	}
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()
	for _, name := range names {
		fmt.Fprintf(w, "  %s\t%s\n", name, cmds[name].describe())
	}
}

// argRef matches references to arguments in run: $1, ${1}, $@, $*, $#.
var argRef = regexp.MustCompile(`\$\{?[1-9@*#]`)

// script appends "$@" to a one-line command that does not reference its
// arguments, so they go at the end. Multi-line scripts and commands that use
// $1 and the like place their arguments themselves.
func script(run string) string {
	run = strings.TrimSpace(run)
	if strings.Contains(run, "\n") || argRef.MatchString(run) {
		return run
	}
	return run + ` "$@"`
}

func runCustom(root, name string, c customCmd, args []string) error {
	if c.Confirm {
		line := strings.TrimSpace(c.Run + " " + strings.Join(args, " "))
		if !confirm("Run " + line + "?") {
			return canceled("Run")
		}
	}
	// Arguments are $1.. / "$@"; $0 is the command name.
	ex := exec.Command("sh", "-c", script(c.Run), name)
	ex.Args = append(ex.Args, args...)
	if c.Dir != "cwd" {
		ex.Dir = root
	}
	pwd, _ := os.Getwd()
	ex.Env = append(os.Environ(), "DOTS_ROOT="+root, "DOTS_PWD="+pwd)
	ex.Stdin, ex.Stdout, ex.Stderr = os.Stdin, os.Stdout, os.Stderr

	err := ex.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return exitCodeError(ee.ExitCode())
	}
	return err
}
