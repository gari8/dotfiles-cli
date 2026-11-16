#!/bin/sh
# Install dots from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/gari8/dotfiles-cli/main/install.sh | sh
#
# Environment:
#   DOTS_VERSION      version to install, e.g. 0.1.0 (default: latest)
#   DOTS_INSTALL_DIR  where to put dots (default: ~/.local/bin)
#   DOTS_BASE_URL     download base URL (default: GitHub Releases; for mirrors and tests)
set -eu

repo="gari8/dotfiles-cli"
install_dir="${DOTS_INSTALL_DIR:-$HOME/.local/bin}"

fail() { echo "install.sh: $*" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported OS: $(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

version="${DOTS_VERSION:-}"
if [ -z "$version" ]; then
  # The latest release URL redirects to .../releases/tag/v<version>.
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
    fail "cannot find the latest release"
  version="${url##*/}"
fi
version="${version#v}"
[ -n "$version" ] && [ "$version" != latest ] || fail "no release found"

base="${DOTS_BASE_URL:-https://github.com/$repo/releases/download/v$version}"
archive="dotfiles-cli_${version}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading dots $version ($os/$arch)"
curl -fsSL "$base/$archive" -o "$tmp/$archive" || fail "download failed: $base/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "download failed: $base/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "$archive is not in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
[ "$actual" = "$expected" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" dots
mkdir -p "$install_dir"
# Replace atomically so a running dots is not broken.
mv -f "$tmp/dots" "$install_dir/dots.new"
chmod +x "$install_dir/dots.new"
mv -f "$install_dir/dots.new" "$install_dir/dots"
echo "Installed $("$install_dir/dots" --version) to $install_dir/dots"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to PATH, e.g.: export PATH=\"$install_dir:\$PATH\"" ;;
esac

echo "Next: clone your dotfiles to ~/dotfiles and run: dots install"
