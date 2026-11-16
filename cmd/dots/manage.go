package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// target records which of --mise / --brew / --cask was given.
type target struct {
	mise, brew, cask bool
}

// addTargetFlags registers --mise / --brew (and --cask). At most one may be
// given; with required, exactly one.
func addTargetFlags(cmd *cobra.Command, t *target, withCask, required bool) {
	cmd.Flags().BoolVar(&t.mise, "mise", false, "mise tools")
	cmd.Flags().BoolVar(&t.brew, "brew", false, "Homebrew (for list / upgrade: formulae, casks and mas apps)")
	opts := "--mise / --brew"
	if withCask {
		cmd.Flags().BoolVar(&t.cask, "cask", false, "Homebrew casks")
		opts += " / --cask"
	}
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		n := 0
		for _, on := range []bool{t.mise, t.brew, t.cask} {
			if on {
				n++
			}
		}
		if n > 1 {
			return fmt.Errorf("give only one of %s", opts)
		}
		if required && n == 0 {
			return fmt.Errorf("give one of %s", opts)
		}
		return nil
	}
}

// all reports that no target was given (list / upgrade then cover everything).
func (t target) all() bool { return !t.mise && !t.brew && !t.cask }

func newAddCmd() *cobra.Command {
	var t target
	var comment string
	cmd := &cobra.Command{
		Use:   "add (--mise|--brew|--cask) <name>...",
		Short: "Install tools and add them to the config",
		Example: `  dots add --mise lazygit terraform@1.9
  dots add --cask arc -m "browser"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			if t.mise {
				if comment != "" {
					return fmt.Errorf("-m works only with --brew / --cask")
				}
				if err := requireTool("mise"); err != nil {
					return err
				}
				return miseEdit(r, "use", args)
			}
			if err := requireTool("brew"); err != nil {
				return err
			}
			return brewAdd(r, kindOf(t.cask), args, comment)
		},
	}
	addTargetFlags(cmd, &t, true, true)
	cmd.Flags().StringVarP(&comment, "comment", "m", "", "comment at the end of the Brewfile line")
	return cmd
}

func newRmCmd() *cobra.Command {
	var t target
	cmd := &cobra.Command{
		Use:     "rm (--mise|--brew|--cask) <name>...",
		Aliases: []string{"remove"},
		Short:   "Remove tools from the config (asks before uninstalling)",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			if t.mise {
				if err := requireTool("mise"); err != nil {
					return err
				}
				return miseEdit(r, "unuse", args)
			}
			if err := requireTool("brew"); err != nil {
				return err
			}
			return brewRemove(r, kindOf(t.cask), args)
		},
	}
	addTargetFlags(cmd, &t, true, true)
	return cmd
}
