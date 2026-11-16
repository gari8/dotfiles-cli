// dots manages a dotfiles repository: symlinks, Homebrew and mise tools,
// and user-defined commands, from anywhere on the machine.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// version is set at build time by goreleaser (-X main.version=...).
var version = "dev"

// assumeYes answers every confirmation prompt with yes (--yes / -y).
var assumeYes bool

func main() {
	root := &cobra.Command{
		Use:           "dots",
		Short:         "Manage your dotfiles",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVarP(&assumeYes, "yes", "y", false, "answer yes to all prompts")

	// The completion script is only sourced by the shell, and help is covered
	// by --help, so neither is listed. cobra's usage template always lists
	// "help", so that condition is removed from the template.
	root.CompletionOptions.HiddenDefaultCmd = true
	root.SetUsageTemplate(strings.ReplaceAll(root.UsageTemplate(),
		`(or .IsAvailableCommand (eq .Name "help"))`, `.IsAvailableCommand`))

	root.AddCommand(
		newInitCmd(),
		newInstallCmd(),
		newAddCmd(),
		newRmCmd(),
		newListCmd(),
		newUpgradeCmd(),
		newPathCmd(),
		newCmdCmd(),
	)

	if err := root.Execute(); err != nil {
		// A custom command's exit status is passed through as is;
		// the command has already printed its own error.
		var code exitCodeError
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		if !errors.Is(err, errCanceled) {
			fmt.Fprintln(os.Stderr, "dots:", err)
		}
		os.Exit(1)
	}
}
