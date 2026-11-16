# dotfiles-cli

`dots` keeps a Mac set up from one dotfiles repository: it links your config files into place, installs your apps and tools, and runs your own commands for the repository from anywhere.

- **GUI apps** come from **Homebrew** (casks, Mac App Store apps)
- **CLI tools and runtimes** come from **mise**
- Formulae that mise does not provide (system tools, libraries) also come from Homebrew

Install Homebrew yourself first; `dots install` installs mise with it. (On Linux the CLI side works; casks and mas apps are macOS only.)

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

For a real example, see the author's dotfiles: [gari8/dotfiles](https://github.com/gari8/dotfiles).

## Install dots

Before you start:

1. The Command Line Tools: `xcode-select --install`
2. Homebrew: the one-line installer on [brew.sh](https://brew.sh); run the lines it prints at the end so `brew` is on your PATH
3. If you will use Mac App Store apps (`mas` lines), sign in to the App Store

Then:

```sh
curl -fsSL https://raw.githubusercontent.com/gari8/dotfiles-cli/main/install.sh | sh
```

This puts `dots` in `~/.local/bin`. Until that is on your `PATH`, type `~/.local/bin/dots` instead of `dots`; `dots install` tells you the line to add. To update `dots` later, run `dots upgrade --self` (or the same line again).

## Getting started

### Starting from scratch

```sh
dots init        # creates ~/dotfiles as a new git repository (asks first)
dots install     # installs Homebrew and mise
```

Then add apps and tools ([Everyday use](#everyday-use)) and put your config files in the repository ([Keep a config file in the repository](RECIPES.md#keep-a-config-file-in-the-repository)).

### Setting up a new machine

Your dotfiles repository already uses dots:

```sh
dots init --from https://github.com/you/dotfiles.git   # clones it to ~/dotfiles
                                                       # (git@github.com:you/dotfiles.git once you have an SSH key)
dots install
```

`--from` is for your own dotfiles (or your fork): before linking, dots shows which files it would link and which custom commands the repository defines, and asks. To borrow parts of someone else's setup, see [Recipes](RECIPES.md) instead.

A fresh machine has no SSH key yet, so use the HTTPS URL: a public repository needs no login. For a private one, set up GitHub access first (an SSH key added to GitHub, or `gh auth login`).

### Moving existing dotfiles to dots

Clone your repository to `~/dotfiles`, then point dots at the files you already have:

```sh
cd ~/dotfiles
dots init --brewfile-gui ./Brewfile --mise-config ./mise.toml
dots install
```

`dots install -n` shows what would happen first. See [Use your existing files](RECIPES.md#use-your-existing-files) for the details.

### Using someone else's setup

Want a starting point? The author's [gari8/dotfiles](https://github.com/gari8/dotfiles) works with any of the options below (replace `someone` with `gari8`).

**All of it:** fork their repository on GitHub, then set up from your fork as in [Setting up a new machine](#setting-up-a-new-machine). Delete what you do not want from the Brewfiles and the mise config before `dots install`, which installs everything in them.

**Parts of it:** make your own `~/dotfiles`, copying the files you want from theirs, take the custom commands you want, then install:

```sh
dots init \
  --brewfile-gui https://github.com/someone/dotfiles.git//brew/gui.Brewfile \
  --mise-config https://github.com/someone/dotfiles.git//config/mise/config.toml
dots pick https://github.com/someone/dotfiles.git commit push   # shown one by one before adding
dots install
```

Leave out the flags you do not need (`--brewfile-cli` copies their formulae). Copying replaces an existing file of yours after asking; to take only a few entries, read theirs and `dots add` them one by one.

### Then: your shell

At the end, `dots install` checks your shell startup files and prints any lines still needed so a new terminal finds `dots`, Homebrew and mise, for example:

```
~/.zprofile    export PATH="$HOME/.local/bin:$PATH"
~/.zprofile    eval "$(/opt/homebrew/bin/brew shellenv)"
~/.zshrc       eval "$(mise activate zsh)"
```

Add them, open a new terminal, and you are done. Keeping those files in your dotfiles ([Keep a config file in the repository](RECIPES.md#keep-a-config-file-in-the-repository)) sets up the next machine too.

## Everyday use

| I want to ... | Run |
| --- | --- |
| Add a GUI app | `dots add --cask <name>` (`-m "note"` adds a comment) |
| Add a CLI tool | `dots add --mise <name>` (`<name>@<version>` to pin) |
| Add a formula mise does not have | `dots add --brew <name>` |
| Remove something | `dots rm --cask\|--mise\|--brew <name>` (asks before uninstalling) |
| See what is managed and installed | `dots list` (`--brew` / `--mise` for one side) |
| Upgrade | `dots upgrade` (shows the list, dots itself included, and asks; `-n` only shows, `-x a,b` leaves some out) |
| Update dots itself | `dots upgrade --self` |
| Check for known vulnerabilities | `dots scan` (see below) |
| Apply the repository after editing it by hand | `dots install` |
| Work on the repository | `cd "$(dots path)"`, or a custom command |

`dots add` and `dots rm` edit the files but do not commit; commit when you are ready.

`-y` answers yes to every prompt (except in `dots pick`).

More in [RECIPES.md](RECIPES.md): [sync through GitHub](RECIPES.md#sync-your-dotfiles-through-github), [keep a config file in the repository](RECIPES.md#keep-a-config-file-in-the-repository), [use your existing files](RECIPES.md#use-your-existing-files), [keep tools up to date and safe](RECIPES.md#keep-tools-up-to-date-and-safe), [hold a formula at a version](RECIPES.md#hold-a-formula-at-a-version), [share commands](RECIPES.md#share-commands-with-others).

### Vulnerability scan

`dots scan` lists known vulnerabilities and what you can do about each one:

```
mise
  gh 2.103.3  (2.104.0 available): dots upgrade
    MEDIUM    GHSA-q7gw-24fr-5p69  go.opentelemetry.io/otel/trace v1.44.0  fixed in 1.47.0
  kubectl 1.37.2  (latest): wait for a new kubectl release
    CRITICAL  GO-2026-6612         golang.org/x/net v0.57.0                fixed in 0.60.0

2 vulnerabilities: 1 fixed by dots upgrade, 1 waiting for a new release of the tool, 0 with no fix yet
```

Most findings are in libraries bundled into a tool: "fixed in" is the library's fixed version, which reaches you only when the tool ships a release built with it. So `dots upgrade` helps only where it says so; the rest wait for the tool's authors (or Homebrew).

Brewfile formulae and their dependencies are checked with `brew vulns` (OSV.dev); mise tools with [grype](https://github.com/anchore/grype), run through `mise exec` (the first run downloads its database). Casks and mas apps are not covered. It exits 1 when anything is found.

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
- [examples/dots.toml](examples/dots.toml) has `status`, `commit`, `push`, `pull`, `sync`, `diff` and `log`. Take them with `dots pick` ([Sync your dotfiles through GitHub](RECIPES.md#sync-your-dotfiles-through-github)).

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
