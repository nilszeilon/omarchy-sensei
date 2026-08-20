#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_dir"
mkdir -p "$repo_dir/build"

if command -v go >/dev/null 2>&1 && go version >/dev/null 2>&1; then
  go build -o "$repo_dir/build/omarchy-sensei" ./cmd/omarchy-sensei
elif command -v mise >/dev/null 2>&1; then
  mise exec go@1.24 -- go build -o "$repo_dir/build/omarchy-sensei" ./cmd/omarchy-sensei
else
  echo "Omarchy Sensei needs Go 1.24 or mise to build its local collector." >&2
  exit 1
fi

"$repo_dir/build/omarchy-sensei" setup
hyprctl reload

errors=$(hyprctl configerrors)
if [[ -n "$errors" ]]; then
  printf '%s\n' "$errors" >&2
  exit 1
fi

echo "Omarchy Sensei is observing semantic shortcuts and matching menu actions."
