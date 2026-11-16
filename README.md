# dotfiles-cli

`dots` keeps a Mac (or Linux) set up from one dotfiles repository: it links your config files into place, installs your apps and tools, and runs your own commands for the repository from anywhere.

- **GUI apps** come from **Homebrew** (casks, Mac App Store apps)
- **CLI tools and runtimes** come from **mise**
- Formulae that mise does not provide (system tools, libraries) also come from Homebrew

`dots install` sets up both Homebrew and mise for you.

## What a dotfiles repository looks like

A git repository; `dots init` creates this layout (every path can be changed in `dots.toml`):

```
~/dotfiles/
├── dots.toml                # dots settings: symlinks, custom commands
├── brew/
│   ├── gui.Brewfile         # casks and mas apps   (dots add --cask)
│   └── cli.Brewfile         # formulae             (dots add --brew)
└── config/mise/config.toml  # mise tools           (dots add --mise)
```

Plus whatever config files you want to keep (`.zshrc`, editor settings, ...), linked into your home directory by `[links]` in `dots.toml`.

## Install dots

macOS needs the Command Line Tools first (`xcode-select --install`). Then:

```sh
curl -fsSL https://raw.githubusercontent.com/gari8/dotfiles-cli/main/install.sh | sh
```

This puts `dots` in `~/.local/bin`. If that is not on your `PATH` yet, use `~/.local/bin/dots` for the commands below. To update `dots` later, run the same line again.

## Getting started

### Starting from scratch

```sh
dots init        # creates ~/dotfiles as a new git repository (asks first)
dots install     # installs Homebrew and mise
```

Then add apps and tools ([Everyday use](#everyday-use)) and put your config files in the repository ([Keep a config file in the repository](#keep-a-config-file-in-the-repository)).

### Setting up a new machine

Your dotfiles repository already uses dots:

```sh
dots init --from https://github.com/you/dotfiles.git   # clones it to ~/dotfiles
                                                       # (git@github.com:you/dotfiles.git once you have an SSH key)
dots install
```

`--from` is for your own dotfiles (or your fork): before linking, dots shows which files it would link and which custom commands the repository defines, and asks. To borrow parts of someone else's setup, see [Recipes](#recipes) instead.

A fresh machine has no SSH key yet, so use the HTTPS URL: a public repository needs no login. For a private one, set up GitHub access first (an SSH key added to GitHub, or `gh auth login`).

### Moving existing dotfiles to dots

Clone your repository to `~/dotfiles`, then point dots at the files you already have:

```sh
cd ~/dotfiles
dots init --brewfile-gui ./Brewfile --mise-config ./mise.toml
dots install
```

`dots install -n` shows what would happen first. See [Use your existing files](#use-your-existing-files) for the details.

## Everyday use

| I want to ... | Run |
| --- | --- |
| Add a GUI app | `dots add --cask <name>` (`-m "note"` adds a comment) |
| Add a CLI tool | `dots add --mise <name>` (`<name>@<version>` to pin) |
| Add a formula mise does not have | `dots add --brew <name>` |
| Remove something | `dots rm --cask\|--mise\|--brew <name>` (asks before uninstalling) |
| See what is managed and installed | `dots list` (`--brew` / `--mise` for one side) |
| Upgrade | `dots upgrade` (shows the list and asks; `-n` only shows, `-x a,b` leaves some out) |
| Check for known vulnerabilities | `dots scan` (see below) |
| Apply the repository after editing it by hand | `dots install` |
| Work on the repository | `cd "$(dots path)"`, or a custom command |

`dots add` and `dots rm` edit the files but do not commit; commit when you are ready.

`-y` answers yes to every prompt (except in `dots pick`).

### Vulnerability scan

`dots scan` lists known vulnerabilities and whether a fixed version is out:

```
mise
  gh 2.103.3  (2.104.0 available: dots upgrade)
    MEDIUM  GHSA-q7gw-24fr-5p69  go.opentelemetry.io/otel/trace v1.44.0  fixed in 1.47.0
```

Brewfile formulae and their dependencies are checked with `brew vulns` (OSV.dev); mise tools with [grype](https://github.com/anchore/grype), run through `mise exec` (the first run downloads its database). Casks and mas apps are not covered. It exits 1 when anything is found.

## Recipes

### Sync your dotfiles through GitHub

`dots pick` takes ready-made git commands from [examples/dots.toml](examples/dots.toml); each one is shown before it is added (once):

```sh
dots pick https://github.com/gari8/dotfiles-cli.git//examples/dots.toml status commit push pull sync
# any git URL works, e.g. git@github.com:gari8/dotfiles-cli.git//examples/dots.toml
```

`~/dotfiles` is already a git repository (`dots init` made it one). Create an empty repository on GitHub and connect it (once):

```sh
cd ~/dotfiles && git remote add origin git@github.com:you/dotfiles.git
```

Pushing needs GitHub access: an SSH key added to GitHub for a `git@github.com:` URL, or `gh auth login` for an `https://` one.

Then, from any directory:

```sh
dots add --cask arc            # change something
dots cmd commit add arc        # shows the diff, asks, commits
dots cmd push                  # shows what will be pushed, asks (sets the upstream the first time)
```

On your other machines, `dots cmd sync` pulls and runs `dots install`.

### Keep a config file in the repository

Move the file into the repository, add a line under `[links]` in `dots.toml`, then run `dots install`:

```sh
mkdir -p ~/dotfiles/zsh && mv ~/.zshrc ~/dotfiles/zsh/.zshrc
```

```toml
[links]
"zsh/.zshrc" = "~/.zshrc"   # file in the repository = link target
```

Anything already at a link target is moved to `~/.dotfiles_backup/<time>/` first.

### Use your existing files

`dots init --brewfile-gui <file>`, `--brewfile-cli <file>` and `--mise-config <file>` make dots use files you already have; the paths are relative to the current directory and are saved under `[paths]` in `dots.toml`. For one Brewfile with everything, give the same file to both Brewfile flags.

### Keep tools up to date and safe

```sh
dots upgrade -n    # what has a newer version
dots upgrade       # upgrade after a yes (-x name to leave something out)
dots scan          # known vulnerabilities, and whether a fixed version is out
```

### Hold a formula at a version

A Brewfile cannot pin versions. `brew pin <name>` holds it on this machine (`dots upgrade` leaves pinned formulae alone). To get the same version on every machine, keep the formula in your own tap and use it from the CLI Brewfile:

```ruby
# brew/cli.Brewfile
tap "you/tap"                  # github.com/you/homebrew-tap
brew "you/tap/opencascade"
```

### Share commands with others

Put commands in a repository (split them with `include` if you like); others take what they want with `dots pick <git url>`, after reading each one.

### Start from someone else's apps

```sh
dots init --brewfile-gui https://github.com/someone/dotfiles.git//brew/gui.Brewfile
```

copies their GUI Brewfile into yours; edit it, then `dots install`.

## Custom commands

Commands for the repository, run with `dots cmd <name> [args]` from any directory (`dots cmd list` shows them):

```toml
[cmd.push]
desc = "Push"              # shown by dots cmd list
run = "git push"           # a shell command, run in the repository
confirm = true             # ask before running (-y skips)
# dir = "cwd"              # run where you called dots instead of in the repository
```

- Arguments go at the end of a one-line `run`. A multi-line `run`, or one that uses `$1`, `"$@"` or `"$*"`, places them itself.
- `$DOTS_ROOT` is the repository and `$DOTS_PWD` is where you ran `dots`.
- `list`, `ls`, `add`, `rm` and `remove` are reserved names.
- [examples/dots.toml](examples/dots.toml) has `status`, `commit`, `push`, `pull`, `sync`, `diff` and `log`. Take them with `dots pick` as shown above.

`dots pick <source> [name...]` copies commands from any `dots.toml`: a local file or directory, or `<git url>[//<path>][?ref=<ref>]` (default path `dots.toml`). They run as shell scripts on your machine, so each one is shown in full and needs a yes, even with `-y`.

When the file grows, move commands to other files and include them:

```toml
include = ["cmds/*.toml"]   # at the top, before any [table]; these files hold only [cmd.*]
```

## dots.toml reference

```toml
version = 1
include = ["cmds/*.toml"]          # more [cmd.*] tables

[paths]                            # relative to the repository (absolute paths work too)
brewfile_cli = "brew/cli.Brewfile"
brewfile_gui = "brew/gui.Brewfile"
mise_config = "config/mise/config.toml"

[links]                            # "file in the repository" = "link target"
"zsh/.zshrc" = "~/.zshrc"

[cmd.<name>]
run = "..."
desc = "..."
dir = "root"                       # or "cwd"
confirm = false
```

Everything is optional; the `[paths]` values above are the defaults.

## Where dots keeps things

| Path | What |
| --- | --- |
| `~/.config/dots/config.toml` | Where your repository is (`root`), written by `dots init` (`$XDG_CONFIG_HOME` is honored) |
| `~/.dotfiles_backup/<time>/` | Files that were in the way of a link |
| `~/.local/bin/dots` | The `dots` binary from `install.sh` |

`dots` finds the repository from `$DOTS_ROOT`, then the file above, then `~/dotfiles`.

## Shell completion

```sh
source <(dots completion zsh)   # in ~/.zshrc
```

## Other ways to install dots

- `go install github.com/gari8/dotfiles-cli/cmd/dots@latest`
- A binary from [Releases](https://github.com/gari8/dotfiles-cli/releases)
- `install.sh` takes `DOTS_VERSION` (e.g. `0.1.3`) and `DOTS_INSTALL_DIR`

## Releasing (for maintainers)

Bump `version` in `version.toml` and push to `main`. GitHub Actions tags `v<version>` and runs goreleaser; nothing happens if the tag already exists.
