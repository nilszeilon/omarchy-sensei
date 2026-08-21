#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_dir"
python3 "$repo_dir/sensei.py" setup
hyprctl reload

errors=$(hyprctl configerrors)
if [[ -n "$errors" ]]; then
  printf '%s\n' "$errors" >&2
  exit 1
fi

echo "Omarchy Sensei is ready: mouse habits become tasks, shortcuts clear them."
