#!/usr/bin/env bash
# kernl installer.
#
#   curl -fsSL https://raw.githubusercontent.com/gabrielassisxyz/kernl/master/install.sh | bash
#
# Downloads the latest release archive for your OS/arch from GitHub and
# installs the `kernl` binary into ~/.local/bin (override with KERNL_BIN_DIR).
# Pin a version with KERNL_VERSION=v0.1.0.
#
# Windows is not a release target — run kernl via Docker there
# (see the Dockerfile / compose.yaml in the repo).
set -euo pipefail
umask 022

REPO="gabrielassisxyz/kernl"
BIN_DIR="${KERNL_BIN_DIR:-$HOME/.local/bin}"
VERSION="${KERNL_VERSION:-latest}"

err()  { printf '\033[31merror:\033[0m %s\n' "$1" >&2; exit 1; }
info() { printf '\033[1m==>\033[0m %s\n' "$1"; }

# --- detect platform (must match goreleaser's archive name_template) ---
os=$(uname -s)
case "$os" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *) err "unsupported OS: $os (Windows users: use the Docker image)" ;;
esac

arch=$(uname -m)
case "$arch" in
    x86_64|amd64)  arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) err "unsupported architecture: $arch" ;;
esac

command -v curl >/dev/null 2>&1 || err "curl is required"
command -v tar  >/dev/null 2>&1 || err "tar is required"

# --- resolve version ---
if [ "$VERSION" = "latest" ]; then
    info "resolving latest release"
    # Primary: GitHub API. Falls back to the redirect target of the
    # /releases/latest page, which does not consume the API's 60/hr
    # unauthenticated rate limit (often exhausted behind shared/CI IPs).
    VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | grep -m1 '"tag_name"' | cut -d'"' -f4)
    if [ -z "$VERSION" ]; then
        VERSION=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
            "https://github.com/$REPO/releases/latest" | sed -E 's#.*/tag/##')
    fi
    [ -n "$VERSION" ] || err "could not resolve the latest release tag"
fi

# Strip a leading v for the archive name (goreleaser uses the bare version).
ver_no_v="${VERSION#v}"
asset="kernl_${ver_no_v}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$VERSION/$asset"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

info "downloading $asset ($VERSION)"
curl -fsSL "$url" -o "$tmp/kernl.tar.gz" \
    || err "download failed: $url"

# Verify the checksum. goreleaser always publishes checksums.txt, so a
# missing file or asset line is treated as a hard failure rather than
# silently installing an unverified binary.
info "verifying checksum"
curl -fsSL "https://github.com/$REPO/releases/download/$VERSION/checksums.txt" -o "$tmp/checksums.txt" \
    || err "could not download checksums.txt for $VERSION; refusing to install unverified binary"
expected=$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || err "no checksum listed for $asset in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/kernl.tar.gz" | awk '{print $1}')
else
    actual=$(shasum -a 256 "$tmp/kernl.tar.gz" | awk '{print $1}')
fi
[ "$expected" = "$actual" ] || err "checksum mismatch (expected $expected, got $actual)"

info "extracting"
tar -xzf "$tmp/kernl.tar.gz" -C "$tmp"
[ -f "$tmp/kernl" ] || err "archive did not contain a kernl binary"

mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/kernl" "$BIN_DIR/kernl"
info "installed kernl $VERSION to $BIN_DIR/kernl"

case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *) printf '\033[33mnote:\033[0m %s is not on your PATH. Add it:\n  export PATH="%s:$PATH"\n' "$BIN_DIR" "$BIN_DIR" ;;
esac

"$BIN_DIR/kernl" --help >/dev/null 2>&1 && info "run 'kernl doctor' to verify your setup." || true
