# Recipes

Ways to use `dots` for common tasks. See the [README](README.md) to get started.

## Sync your dotfiles through GitHub

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

## Keep a config file in the repository

Move the file into the repository, add a line under `[links]` in `dots.toml`, then run `dots install`:

```sh
mkdir -p ~/dotfiles/zsh && mv ~/.zshrc ~/dotfiles/zsh/.zshrc
```

```toml
[links]
"zsh/.zshrc" = "~/.zshrc"   # file in the repository = link target
```

Anything already at a link target is moved to `~/.dotfiles_backup/<time>/` first.

## Use your existing files

`dots init --brewfile-gui <file>`, `--brewfile-cli <file>` and `--mise-config <file>` make dots use files you already have; the paths are relative to the current directory and are saved under `[paths]` in `dots.toml`. For one Brewfile with everything, give the same file to both Brewfile flags.

## Keep tools up to date and safe

```sh
dots upgrade -n    # what has a newer version (dots itself too)
dots upgrade       # upgrade after a yes (-x name to leave something out)
dots scan          # known vulnerabilities, and whether a fixed version is out
```

## Hold a formula at a version

A Brewfile cannot pin versions. `brew pin <name>` holds it on this machine (`dots upgrade` leaves pinned formulae alone). To get the same version on every machine, keep the formula in your own tap and use it from the CLI Brewfile:

```ruby
# brew/cli.Brewfile
tap "you/tap"                  # github.com/you/homebrew-tap
brew "you/tap/opencascade"
```

## Share commands with others

Put commands in a repository (split them with `include` if you like); others take what they want with `dots pick <git url>`, after reading each one.
