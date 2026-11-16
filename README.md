# dotfiles-cli

`dots` manages a dotfiles repository from anywhere: symlinks, tools, and your own commands.

GUI apps come from **Homebrew** (Brewfile) and CLI tools and runtimes from **mise**, so `dots` needs both; `dots install` sets them up.

## Set up a new machine

```sh
git clone <your dotfiles> ~/dotfiles
curl -fsSL https://raw.githubusercontent.com/gari8/dotfiles-cli/main/install.sh | sh
~/.local/bin/dots install
```

`dots install` creates the symlinks under `[links]`, installs Homebrew if needed (after asking) and runs `brew bundle`, installs mise with Homebrew if needed and runs `mise install`. Safe to run again.

Other ways to install `dots`: `mise use -g github:gari8/dotfiles-cli`, `go install github.com/gari8/dotfiles-cli/cmd/dots@latest`, or [Releases](https://github.com/gari8/dotfiles-cli/releases). `install.sh` takes `DOTS_VERSION` and `DOTS_INSTALL_DIR`.

Starting from scratch or outside `~/dotfiles`? `dots init [dir]` creates `dots.toml`, a Brewfile and a mise config. Shell completion (zsh): `source <(dots completion zsh)`.

## Commands

| Command | Does |
| --- | --- |
| `dots install` | Links, Homebrew + `brew bundle`, `mise install` (`-n` to preview) |
| `dots init [dir]` | Register the repository (default `~/dotfiles`), create missing config files, create symlinks |
| `dots add --mise\|--brew\|--cask <name>` | Install and add to the mise config / Brewfile |
| `dots rm --mise\|--brew\|--cask <name>` | Remove from the config (asks before uninstalling) |
| `dots list [--mise\|--brew]` | Show managed tools and whether they are installed |
| `dots upgrade [--mise\|--brew]` | Show available upgrades and apply them (`-n` preview, `-x a,b` exclude) |
| `dots cmd <name> [args]` | Run a custom command (`dots cmd list`) |
| `dots path` | Print the repository path (`cd "$(dots path)"`) |

`-y` answers yes to every prompt.

The repository is found from `$DOTS_ROOT`, then `root` in `~/.config/dots/config.toml` (written by `dots init`), then `~/dotfiles`.

## dots.toml

At the root of the dotfiles repository. Everything is optional. See [examples/dots.toml](examples/dots.toml) for a full example with git commands.

```toml
version = 1

[paths]                      # relative to the repository
brewfile = "Brewfile"
mise_config = "config/mise/config.toml"

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

Bump `version` in `version.toml` and push to `main`. GitHub Actions tags `v<version>` and runs goreleaser (nothing happens if the tag already exists). Pushing a `v*` tag by hand also releases.
