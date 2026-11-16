package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// remoteSpec is a file in a git repository, written as
//
//	<git url>[//<path>][?ref=<branch or tag>]
//
// for example git@github.com:gari8/dotfiles.git//Brewfile.
type remoteSpec struct {
	repo, path, ref string
}

// gitURLPrefixes mark a value as a git URL rather than a local path.
var gitURLPrefixes = []string{"git@", "ssh://", "https://", "http://", "git://", "file://"}

func isRemote(s string) bool {
	for _, p := range gitURLPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// parseRemote splits a remote spec. defaultPath is used when there is no //<path>.
func parseRemote(s, defaultPath string) remoteSpec {
	var spec remoteSpec
	if i := strings.LastIndex(s, "?ref="); i >= 0 {
		s, spec.ref = s[:i], s[i+len("?ref="):]
	}
	// Look for // after the scheme's own "://".
	start := 0
	if i := strings.Index(s, "://"); i >= 0 {
		start = i + len("://")
	}
	if i := strings.Index(s[start:], "//"); i >= 0 {
		spec.repo, spec.path = s[:start+i], strings.TrimPrefix(s[start+i+2:], "/")
	} else {
		spec.repo = s
	}
	if spec.path == "" {
		spec.path = defaultPath
	}
	return spec
}

func (s remoteSpec) String() string {
	out := s.repo + "//" + s.path
	if s.ref != "" {
		out += "?ref=" + s.ref
	}
	return out
}

// gitClone clones repo (shallow, at ref if given) into dir.
func gitClone(repo, ref, dir string, shallow bool) error {
	args := []string{"clone", "--quiet"}
	if shallow {
		args = append(args, "--depth", "1")
	}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, repo, dir)
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", repo, err)
	}
	return nil
}

// fetchRemote returns the contents of the file a remote spec points to.
func fetchRemote(spec remoteSpec) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "dots-fetch-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	dir := filepath.Join(tmp, "repo")
	if err := gitClone(spec.repo, spec.ref, dir, true); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(spec.path)))
	if err != nil {
		return nil, fmt.Errorf("%s: %s not found in the repository", spec, spec.path)
	}
	return b, nil
}
