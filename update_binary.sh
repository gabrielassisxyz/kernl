#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$REPO_ROOT"

TARGET="$HOME/.local/bin/kernl"

# Resolve past the symlink so the build lands on the real file: mv'ing over
# the symlink path itself would replace the symlink with a plain file.
if [ -L "$TARGET" ]; then
    DEST="$(readlink -f "$TARGET")"
else
    DEST="$TARGET"
fi
DEST_DIR="$(dirname "$DEST")"

if [ ! -w "$DEST_DIR" ]; then
    echo "update_binary.sh: $DEST_DIR is not writable" >&2
    exit 1
fi

# Build next to the destination, on the same filesystem, then rename over it.
# rename(2) swaps the directory entry atomically, so a `kernl serve` already
# running keeps the inode it has open and the binary updates without hitting
# "text file busy" on the live executable.
TMP_BIN="$(mktemp "$DEST_DIR/.kernl.XXXXXX")"
trap 'rm -f "$TMP_BIN"' EXIT

go build -o "$TMP_BIN" ./cmd/kernl/
mv -f "$TMP_BIN" "$DEST"

echo "kernl binary updated → $TARGET"
