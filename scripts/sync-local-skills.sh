#!/usr/bin/env bash
# Symlink harness-independent revmux skill assets (scripts/ and shared references)
# from this repository into local user skill directories (~/.gemini, ~/.pi) so
# copies never drift while harness-specific SKILL.md and references stay intact.
set -euo pipefail

repo_root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
src_root="$repo_root/.claude-plugin/skills/revmux"

if [ ! -d "$src_root" ]; then
    echo "error: source skill tree not found: $src_root" >&2
    exit 1
fi

if [ "$#" -gt 0 ]; then
    targets=("$@")
else
    targets=(
        "$HOME/.gemini/config/skills/revmux"
        "$HOME/.pi/agent/skills/revmux"
    )
fi

shared_items=(
    "scripts"
    "references/output.md"
    "references/task-dir.md"
)

stamp="$(date +%Y%m%dT%H%M%S)"
backup_root="${TMPDIR:-/tmp}/revmux-skill-sync-backup-${stamp}"

link_item() {
    local target_dir="$1"
    local rel="$2"
    local src="$src_root/$rel"
    local dest="$target_dir/$rel"
    local current=""
    local backup_dest=""

    if [ ! -e "$src" ]; then
        echo "error: missing source item: $src" >&2
        exit 1
    fi

    if [ -L "$dest" ]; then
        current="$(readlink "$dest")"
        if [ "$current" = "$src" ]; then
            echo "ok:   $dest -> $src"
            return 0
        fi
        rm -f "$dest"
    elif [ -e "$dest" ]; then
        backup_dest="$backup_root/${target_dir#/}/$rel"
        mkdir -p "$(dirname -- "$backup_dest")"
        mv "$dest" "$backup_dest"
        echo "bak:  $dest -> $backup_dest"
    fi

    mkdir -p "$(dirname -- "$dest")"
    ln -s "$src" "$dest"
    echo "link: $dest -> $src"
}

for target_dir in "${targets[@]}"; do
    if [ ! -d "$target_dir" ]; then
        echo "skip: $target_dir (not installed)"
        continue
    fi
    for rel in "${shared_items[@]}"; do
        link_item "$target_dir" "$rel"
    done
done
