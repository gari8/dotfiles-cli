package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// homebrewInstaller is Homebrew's official install command.
const homebrewInstaller = `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`

func newInstallCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Set up this machine: links, Homebrew + Brewfile, mise tools",
		Long: `Set up this machine from the dotfiles repository. GUI apps come from
Homebrew and CLI tools from mise, so both are required:

  1. create the symlinks under [links]
  2. install Homebrew if missing (after asking), then brew bundle the Brewfile
  3. install mise with Homebrew if missing, then mise install

Safe to run again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := openRepo()
			if err != nil {
				return err
			}
			return install(r, dryRun)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "only show what would be done")
	return cmd
}

func install(r *repo, dryRun bool) error {
	if dryRun {
		fmt.Println("(dry run: nothing is changed)")
	}

	fmt.Println("== links ==")
	if len(r.cfg.Links) == 0 {
		fmt.Println("  skip    no [links] in " + configFile)
	} else if err := applyLinks(r.root, r.cfg.Links, dryRun); err != nil {
		return err
	}

	fmt.Println("\n== Homebrew ==")
	if err := installBrew(r, dryRun); err != nil {
		return err
	}

	fmt.Println("\n== mise ==")
	if err := installMise(r, dryRun); err != nil {
		return err
	}

	if !dryRun {
		fmt.Println("\nDone. Restart your shell (exec $SHELL -l) to pick up the changes.")
	}
	return nil
}

func installBrew(r *repo, dryRun bool) error {
	brewfile := r.brewfile()
	if !exists(brewfile) {
		return fmt.Errorf("Brewfile not found: %s (dots init creates one)", tildePath(brewfile))
	}

	brew := findBrew()
	if brew == "" {
		if dryRun {
			fmt.Println("  install Homebrew (asks first)")
			fmt.Printf("  run     brew bundle install --no-upgrade --file=%s\n", tildePath(brewfile))
			return nil
		}
		if !confirm("Homebrew is not installed. Install it?") {
			return errors.New("dots needs Homebrew for GUI apps; install canceled")
		}
		if err := run(r.root, "/bin/bash", "-c", homebrewInstaller); err != nil {
			return err
		}
		if brew = findBrew(); brew == "" {
			return errors.New("Homebrew was installed but brew was not found")
		}
	}
	// Make brew and what it installs (mise, for one) visible to later steps.
	prependPath(filepath.Dir(brew))

	// Only install what is missing; upgrades are left to dots upgrade, which asks first.
	if dryRun {
		fmt.Printf("  run     brew bundle install --no-upgrade --file=%s\n", tildePath(brewfile))
		return nil
	}
	return run(r.root, brew, "bundle", "install", "--no-upgrade", "--file="+brewfile)
}

func installMise(r *repo, dryRun bool) error {
	config := r.miseConfig()
	if !exists(config) {
		return fmt.Errorf("mise config not found: %s (dots init creates one)", tildePath(config))
	}
	if !have("mise") {
		fmt.Println("  run     brew install mise")
		if !dryRun {
			if err := run(r.root, "brew", "install", "mise"); err != nil {
				return err
			}
		}
	}
	fmt.Println("  run     mise install")
	if dryRun {
		return nil
	}
	return run(r.root, "env", miseEnv(r), "mise", "install")
}

// findBrew returns the brew executable: on PATH, or where the installer puts it.
func findBrew() string {
	if p, err := exec.LookPath("brew"); err == nil {
		return p
	}
	for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew", "/home/linuxbrew/.linuxbrew/bin/brew"} {
		if isExecutable(p) {
			return p
		}
	}
	return ""
}

func prependPath(dir string) {
	path := os.Getenv("PATH")
	for _, d := range filepath.SplitList(path) {
		if d == dir {
			return
		}
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+path)
}

// have reports whether a command is on PATH.
func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
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

// requireTool returns an error when brew or mise is missing.
func requireTool(name string) error {
	if have(name) {
		return nil
	}
	return fmt.Errorf("%s is not installed (run dots install)", name)
}
