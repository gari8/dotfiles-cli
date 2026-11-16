package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Run the repository's setup script (paths.install in dots.toml)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			path := r.path(r.cfg.Paths.Install)
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("setup script not found: %s", tildePath(path))
			}
			var extra []string
			if dryRun {
				extra = append(extra, "--dry-run")
			}
			return run(r.root, path, extra...)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "pass --dry-run to the script")
	return cmd
}

func newPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "path",
		Short:   "Print the dotfiles repository path",
		Example: `  cd "$(dots path)"`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			fmt.Println(root)
			return nil
		},
	}
}
