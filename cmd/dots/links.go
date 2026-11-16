package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// applyLinks creates the symlinks in links (repo-relative source -> target).
// A correct link is left alone; anything else at the target is moved to
// ~/.dotfiles_backup/<time>/ first.
func applyLinks(root string, links map[string]string, dryRun bool) error {
	if len(links) == 0 {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(home, ".dotfiles_backup", time.Now().Format("20060102-150405"))

	sources := make([]string, 0, len(links))
	for src := range links {
		sources = append(sources, src)
	}
	sort.Strings(sources)

	for _, src := range sources {
		from := filepath.Join(root, src)
		to, err := expandHome(links[src])
		if err != nil {
			return err
		}
		if err := link(from, to, backupDir, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func link(from, to, backupDir string, dryRun bool) error {
	if _, err := os.Stat(from); err != nil {
		fmt.Printf("  skip    %s (source missing: %s)\n", tildePath(to), tildePath(from))
		return nil
	}
	if dest, err := os.Readlink(to); err == nil && dest == from {
		fmt.Printf("  ok      %s\n", tildePath(to))
		return nil
	}

	// Anything else at the target (file, directory, other link) is backed up.
	if _, err := os.Lstat(to); err == nil {
		backup := uniquePath(filepath.Join(backupDir, filepath.Base(to)))
		fmt.Printf("  backup  %s -> %s\n", tildePath(to), tildePath(backup))
		if !dryRun {
			if err := os.MkdirAll(backupDir, 0o755); err != nil {
				return err
			}
			if err := os.Rename(to, backup); err != nil {
				return err
			}
		}
	}

	fmt.Printf("  link    %s -> %s\n", tildePath(to), tildePath(from))
	if dryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Symlink(from, to)
}

// uniquePath adds .1, .2, ... when path already exists.
func uniquePath(path string) string {
	candidate := path
	for i := 1; ; i++ {
		if _, err := os.Lstat(candidate); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s.%d", path, i)
	}
}
