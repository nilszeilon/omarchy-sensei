#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_dir"
mkdir -p "$repo_dir/build"

if ! command -v go >/dev/null 2>&1; then
  echo "Omarchy Sensei needs Go 1.24+ installed locally to build its helper." >&2
  exit 1
fi

go_version=$(GOTOOLCHAIN=local go env GOVERSION 2>/dev/null || true)
if [[ ! "$go_version" =~ ^go([0-9]+)\.([0-9]+) ]] ||
   (( BASH_REMATCH[1] < 1 || (BASH_REMATCH[1] == 1 && BASH_REMATCH[2] < 24) )); then
  echo "Omarchy Sensei needs Go 1.24+ installed locally (found ${go_version:-unknown})." >&2
  exit 1
fi

GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
  go build -o "$repo_dir/build/omarchy-sensei" ./cmd/omarchy-sensei

"$repo_dir/build/omarchy-sensei" setup
hyprctl reload

errors=$(hyprctl configerrors)
if [[ -n "$errors" ]]; then
  printf '%s\n' "$errors" >&2
  exit 1
fi

echo "Omarchy Sensei is ready: mouse habits become tasks, shortcuts clear them."
