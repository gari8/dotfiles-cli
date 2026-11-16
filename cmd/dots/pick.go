package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

func newPickCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pick <source> [<name>...]",
		Short: "Copy custom commands from another dots.toml",
		Long: `Copy [cmd.<name>] tables from another dots.toml into yours.

<source> is a dots.toml, a directory containing one, or
<git url>[//<path>][?ref=<ref>] (default path: dots.toml).
Without names, the commands in <source> are listed.

Custom commands run as shell scripts on your machine, so each one is shown in
full and needs a yes; -y does not skip this.`,
		Example: `  dots pick git@github.com:gari8/dotfiles.git
  dots pick git@github.com:gari8/dotfiles.git push commit`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			src, err := readSource(args[0])
			if err != nil {
				return err
			}
			var theirs struct {
				Cmd map[string]customCmd `toml:"cmd"`
			}
			if _, err := toml.Decode(string(src), &theirs); err != nil {
				return fmt.Errorf("%s: %w", args[0], err)
			}
			if len(args) == 1 {
				listCustom(theirs.Cmd)
				fmt.Printf("\nAdd some with: dots pick %s <name>...\n", args[0])
				return nil
			}
			for _, name := range args[1:] {
				c, ok := theirs.Cmd[name]
				if !ok {
					return fmt.Errorf("no custom command %q in %s", name, args[0])
				}
				if err := pickCmd(r, name, c); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// readSource reads a dots.toml from a git repository, a directory or a file.
func readSource(source string) ([]byte, error) {
	if isRemote(source) {
		return fetchRemote(parseRemote(source, configFile))
	}
	path, err := expandHome(source)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		path = filepath.Join(path, configFile)
	}
	return os.ReadFile(path)
}

// pickCmd shows the command, asks, and writes it to the repository's dots.toml.
func pickCmd(r *repo, name string, c customCmd) error {
	if reservedNames[name] {
		return fmt.Errorf("cmd.%s is a reserved name", name)
	}
	// pick writes to dots.toml, so a command from an included file is not replaced.
	if from, ok := r.cfg.cmdFrom[name]; ok && from != configFile {
		return fmt.Errorf("cmd.%s is defined in %s; remove it there first", name, from)
	}
	block := formatCmd(name, c)
	fmt.Printf("\n%s\n", block)
	question := "Add cmd." + name + " to your dots.toml?"
	if _, ok := r.cfg.Cmd[name]; ok {
		question = "Replace your cmd." + name + " with this?"
	}
	// Not confirm(): -y must not let someone else's script in unseen.
	fmt.Printf("This runs as a shell script on your machine. %s [y/N] ", question)
	line, _ := stdin.ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		fmt.Printf("Skipped cmd.%s\n", name)
		return nil
	}

	path := filepath.Join(r.root, configFile)
	before, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	after := strings.TrimRight(removeTable(string(before), "cmd."+name), "\n")
	if after != "" {
		after += "\n\n"
	}
	after += block + "\n"
	if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
		return err
	}
	// Keep the old file if the result does not load.
	if _, err := loadConfig(r.root); err != nil {
		os.WriteFile(path, before, 0o644)
		return fmt.Errorf("cmd.%s was not added: %w", name, err)
	}
	r.cfg.Cmd = ensureMap(r.cfg.Cmd)
	r.cfg.Cmd[name] = c
	if r.cfg.cmdFrom == nil {
		r.cfg.cmdFrom = map[string]string{}
	}
	r.cfg.cmdFrom[name] = configFile
	fmt.Printf("Added cmd.%s (dots cmd %s)\n", name, name)
	return nil
}

func ensureMap(m map[string]customCmd) map[string]customCmd {
	if m == nil {
		return map[string]customCmd{}
	}
	return m
}

// formatCmd writes a [cmd.<name>] table. A multi-line run becomes a literal
// multi-line string so the script stays readable.
func formatCmd(name string, c customCmd) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[cmd.%s]\n", name)
	if c.Desc != "" {
		fmt.Fprintf(&b, "desc = %q\n", c.Desc)
	}
	if c.Dir != "" {
		fmt.Fprintf(&b, "dir = %q\n", c.Dir)
	}
	if c.Confirm {
		b.WriteString("confirm = true\n")
	}
	if strings.Contains(c.Run, "\n") && !strings.Contains(c.Run, "'''") {
		fmt.Fprintf(&b, "run = '''\n%s'''", strings.TrimSuffix(c.Run, "\n")+"\n")
	} else {
		fmt.Fprintf(&b, "run = %q", c.Run)
	}
	return b.String()
}

// tableHeader matches a table header line such as [cmd.push].
var tableHeader = regexp.MustCompile(`^\s*\[[^\[\]]+\]\s*(#.*)?$`)

// removeTable drops the [name] table (header to the next header) from a TOML
// document, ignoring lines inside multi-line strings.
func removeTable(content, name string) string {
	lines := strings.Split(content, "\n")
	inString := false
	start, end := -1, len(lines)
	for i, l := range lines {
		if !inString && tableHeader.MatchString(l) {
			if start >= 0 {
				end = i
				break
			}
			if strings.TrimSpace(strings.SplitN(l, "#", 2)[0]) == "["+name+"]" {
				start = i
			}
		}
		// An odd number of ''' or """ on a line opens or closes a multi-line string.
		if (strings.Count(l, "'''")+strings.Count(l, `"""`))%2 == 1 {
			inString = !inString
		}
	}
	if start < 0 {
		return content
	}
	out := append(append([]string{}, lines[:start]...), lines[end:]...)
	return strings.Join(out, "\n")
}
