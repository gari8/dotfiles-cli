package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// userPath is PATH as dots was started with, before dots adds anything to it.
var userPath = os.Getenv("PATH")

// shellLine is a line the shell setup needs, and the file it goes in.
type shellLine struct {
	file, line string
}

// shellFiles are the startup files checked for each shell, relative to home.
var shellFiles = map[string][]string{
	"zsh":  {".zprofile", ".zshrc", ".zshenv", ".zlogin"},
	"bash": {".bash_profile", ".bashrc", ".profile"},
	"fish": {".config/fish/config.fish"},
}

// missingShellSetup returns what the shell's startup files lack for dots,
// Homebrew and mise to work in a new terminal. brew is the brew executable
// ("" when unknown); pathDirs is the PATH dots was started with.
func missingShellSetup(home, shell, brew string, pathDirs []string, read func(string) string) []shellLine {
	files, ok := shellFiles[shell]
	if !ok {
		return nil
	}
	var all strings.Builder
	for _, f := range files {
		all.WriteString(read(filepath.Join(home, f)))
		all.WriteString("\n")
	}
	content := all.String()
	onPath := func(dir string) bool {
		for _, d := range pathDirs {
			if d == dir {
				return true
			}
		}
		return false
	}

	login, rc := files[0], files[1]
	if shell == "fish" {
		login, rc = files[0], files[0]
	}
	var missing []shellLine
	add := func(file, line string) { missing = append(missing, shellLine{"~/" + file, line}) }

	localBin := filepath.Join(home, ".local", "bin")
	if !onPath(localBin) && !strings.Contains(content, ".local/bin") {
		if shell == "fish" {
			add(login, "fish_add_path ~/.local/bin")
		} else {
			add(login, `export PATH="$HOME/.local/bin:$PATH"`)
		}
	}
	// /usr/local/bin (Intel Macs) is on PATH by default.
	if brew != "" && !onPath(filepath.Dir(brew)) && !strings.Contains(content, "brew shellenv") {
		if shell == "fish" {
			add(login, brew+" shellenv | source")
		} else {
			add(login, fmt.Sprintf(`eval "$(%s shellenv)"`, brew))
		}
	}
	if !strings.Contains(content, "mise activate") {
		if shell == "fish" {
			add(rc, "mise activate fish | source")
		} else {
			add(rc, fmt.Sprintf(`eval "$(mise activate %s)"`, shell))
		}
	}
	return missing
}

// printShellSetup tells the user which lines to add to their shell startup
// files, if any. A startup file that [links] links (or will link, in a dry
// run) is read from the repository.
func printShellSetup(r *repo) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	if _, ok := shellFiles[shell]; !ok {
		fmt.Printf("  skip    %s is not checked; make sure ~/.local/bin and Homebrew are on PATH and mise is activated\n", orUnknown(shell))
		return
	}
	linked := map[string]string{} // link target -> file in the repository
	for src, to := range r.cfg.Links {
		if target, err := expandHome(to); err == nil {
			linked[target] = filepath.Join(r.root, src)
		}
	}
	read := func(path string) string {
		if src, ok := linked[path]; ok {
			path = src
		}
		b, _ := os.ReadFile(path)
		return string(b)
	}
	missing := missingShellSetup(home, shell, findBrew(), filepath.SplitList(userPath), read)
	if len(missing) == 0 {
		fmt.Printf("  ok      %s finds dots, Homebrew and mise\n", shell)
		return
	}
	fmt.Println("  Add these lines so a new terminal finds dots, Homebrew and mise:")
	for _, m := range missing {
		fmt.Printf("    %-14s %s\n", m.file, m.line)
	}
	fmt.Println("  (Keep those files in your dotfiles and link them with [links] to set up the next machine too.)")
}
