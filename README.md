# dotfiles-cli

`dots` manages a dotfiles repository from anywhere: symlinks, Homebrew and mise tools, and your own commands.

## Install

With [mise](https://mise.jdx.dev):

```sh
mise use -g github:gari8/dotfiles-cli
```

Or download a binary from [Releases](https://github.com/gari8/dotfiles-cli/releases), or `go install github.com/gari8/dotfiles-cli/cmd/dots@latest`.

## Setup

```sh
cd ~/dotfiles
dots init        # registers this directory, creates dots.toml, links [links]
```

Shell completion (zsh):

```sh
source <(dots completion zsh)
```

## Commands

| Command | Does |
| --- | --- |
| `dots init [dir]` | Register the repository, create `dots.toml`, create symlinks (`-n` to preview) |
| `dots install` | Run the setup script (`paths.install`) |
| `dots add --mise\|--brew\|--cask <name>` | Install and add to the mise config / Brewfile |
| `dots rm --mise\|--brew\|--cask <name>` | Remove from the config (asks before uninstalling) |
| `dots list [--mise\|--brew]` | Show managed tools and whether they are installed |
| `dots upgrade [--mise\|--brew]` | Show available upgrades and apply them (`-n` preview, `-x a,b` exclude) |
| `dots cmd <name> [args]` | Run a custom command (`dots cmd list`) |
| `dots path` | Print the repository path (`cd "$(dots path)"`) |

`-y` answers yes to every prompt.

The repository is found from `$DOTS_ROOT`, then `root` in `~/.config/dots/config.toml` (written by `dots init`), then `~/dotfiles`.

## dots.toml

At the root of the dotfiles repository. Everything is optional.

```toml
version = 1

[paths]                      # relative to the repository
brewfile = "Brewfile"
mise_config = "config/mise/config.toml"
install = "scripts/install.sh"

[brewfile]                   # dots add puts entries under the matching "# --- ... ---" heading
formula_section = "CLI"
cask_section = "GUI"

[links]                      # file in the repository = link target
"zsh/.zshrc" = "~/.zshrc"

[cmd.push]                   # dots cmd push
run = "git push"
desc = "Push"
confirm = true               # ask first (-y skips)
# dir = "cwd"                # run where dots was called (default: the repository)
```

In custom commands, `$DOTS_ROOT` is the repository and `$DOTS_PWD` is where `dots` was run. A one-line `run` that does not use `$1`, `"$@"` etc. gets the arguments appended. `list`, `ls`, `add`, `rm` and `remove` are reserved names.

## Release

Push a `v*` tag; GitHub Actions runs goreleaser.
