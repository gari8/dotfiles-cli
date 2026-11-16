# dotfiles-cli

`dots` manages a dotfiles repository from anywhere: symlinks, tools, and your own commands.

GUI apps come from **Homebrew** and CLI tools and runtimes from **mise**, so `dots` needs both; `dots install` sets them up. There are two Brewfiles: one for casks and mas apps (GUI) and one for the formulae mise does not provide (CLI).

## Set up a new machine

```sh
curl -fsSL https://raw.githubusercontent.com/gari8/dotfiles-cli/main/install.sh | sh
~/.local/bin/dots init --from git@github.com:you/dotfiles.git   # clones to ~/dotfiles
~/.local/bin/dots install
```

`dots install` creates the symlinks under `[links]`, installs Homebrew if needed (after asking) and runs `brew bundle`, installs mise with Homebrew if needed and runs `mise install`. Safe to run again.

Other ways to install `dots`: `mise use -g github:gari8/dotfiles-cli`, `go install github.com/gari8/dotfiles-cli/cmd/dots@latest`, or [Releases](https://github.com/gari8/dotfiles-cli/releases). `install.sh` takes `DOTS_VERSION` and `DOTS_INSTALL_DIR`.

Starting from scratch or outside `~/dotfiles`? `dots init [dir]` creates `dots.toml`, the two Brewfiles and a mise config. To use files you already have, point at them: `dots init --brewfile-gui ./Brewfile --mise-config ./mise.toml` (saved under `[paths]`; `--brewfile-cli` for formulae). A `<git url>//<path>[?ref=<ref>]` value copies the file from a repository instead, e.g. `--brewfile-gui git@github.com:someone/dotfiles.git//brew/gui.Brewfile`. Shell completion (zsh): `source <(dots completion zsh)`.

## Commands

| Command | Does |
| --- | --- |
| `dots install` | Links, Homebrew + `brew bundle`, `mise install` (`-n` to preview) |
| `dots init [dir]` | Register the repository (default `~/dotfiles`), create missing config files, create symlinks (`--from <git url>` clones first) |
| `dots add --mise\|--brew\|--cask <name>` | Install and add to the mise config / CLI Brewfile / GUI Brewfile |
| `dots rm --mise\|--brew\|--cask <name>` | Remove from the config (asks before uninstalling) |
| `dots list [--mise\|--brew]` | Show managed tools and whether they are installed |
| `dots upgrade [--mise\|--brew]` | Show available upgrades and apply them (`-n` preview, `-x a,b` exclude) |
| `dots scan [--mise\|--brew]` | Check for known vulnerabilities and whether a fix is released (exits 1 when anything is found) |
| `dots cmd <name> [args]` | Run a custom command (`dots cmd list`) |
| `dots pick <source> [name...]` | Copy custom commands from another dots.toml (shows each one and asks, even with `-y`) |
| `dots path` | Print the repository path (`cd "$(dots path)"`) |

`-y` answers yes to every prompt.

`dots scan` uses `brew vulns` (OSV.dev) for the formulae in the Brewfiles and their dependencies, and [grype](https://github.com/anchore/grype) through `mise exec` for mise tools; casks and mas apps are not covered.

The repository is found from `$DOTS_ROOT`, then `root` in `~/.config/dots/config.toml` (written by `dots init`), then `~/dotfiles`.

## dots.toml

At the root of the dotfiles repository. Everything is optional. See [examples/dots.toml](examples/dots.toml) for a full example with git commands.

```toml
version = 1
include = ["cmds/*.toml"]    # more [cmd.*] tables, e.g. cmds/git.toml (before any [table])

[paths]                      # relative to the repository
brewfile_cli = "brew/cli.Brewfile"   # formulae (dots add --brew)
brewfile_gui = "brew/gui.Brewfile"   # casks and mas apps (dots add --cask)
mise_config = "config/mise/config.toml"

[links]                      # file in the repository = link target
"zsh/.zshrc" = "~/.zshrc"

[cmd.push]                   # dots cmd push
run = "git push"
desc = "Push"
confirm = true               # ask first (-y skips)
# dir = "cwd"                # run where dots was called (default: the repository)
```

In custom commands, `$DOTS_ROOT` is the repository and `$DOTS_PWD` is where `dots` was run. A one-line `run` that does not use `$1`, `"$@"` etc. gets the arguments appended. `list`, `ls`, `add`, `rm` and `remove` are reserved names. Included files may only contain `[cmd.*]` tables, and a name may be defined only once.

## Release

Bump `version` in `version.toml` and push to `main`. GitHub Actions tags `v<version>` and runs goreleaser (nothing happens if the tag already exists). Pushing a `v*` tag by hand also releases.
