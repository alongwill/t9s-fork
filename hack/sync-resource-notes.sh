#!/usr/bin/env bash
# Copy the resource notes from the Talos skill into the embedded catalog copy.
#   hack/sync-resource-notes.sh [path]          copy (default: $TALOS_SKILL_DIR or the agent-skills checkout)
#   hack/sync-resource-notes.sh --check [path]  exit 1 if the copy differs (header ignored)
set -euo pipefail

HEADER='# GENERATED from the talos skill (knowledge/resource-notes.yaml). Edit it there, then rerun hack/sync-resource-notes.sh.'
DEFAULT_SKILL="${TALOS_SKILL_DIR:-/Users/andrewlongwill/sources/github.com/alongwill/agent-skills/talos}"

check=0
if [[ "${1:-}" == "--check" ]]; then
	check=1
	shift
fi
src="${1:-$DEFAULT_SKILL/knowledge/resource-notes.yaml}"
dst="$(cd "$(dirname "$0")/.." && pwd)/internal/catalog/resource-notes.yaml"

if [[ ! -f "$src" ]]; then
	echo "source not found: $src" >&2
	exit 2
fi

if [[ $check -eq 1 ]]; then
	if diff -q <(grep -vxF "$HEADER" "$dst") "$src" >/dev/null; then
		echo "resource-notes.yaml is in sync"
		exit 0
	fi
	echo "internal/catalog/resource-notes.yaml differs from $src; run hack/sync-resource-notes.sh" >&2
	exit 1
fi

{
	echo "$HEADER"
	cat "$src"
} >"$dst"
echo "copied $src -> $dst"
