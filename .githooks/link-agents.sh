#!/usr/bin/env bash
# Regenerate the Claude-specific symlinks from the tool-agnostic canonical files.
#   - CLAUDE.md  -> sibling AGENTS.md   (every directory that has one)
#   - .claude    -> .agents             (repo root)
# Idempotent; safe to run repeatedly. Invoked by the git hooks in this dir.
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# Per-directory CLAUDE.md -> AGENTS.md
while IFS= read -r -d '' agents; do
  dir="$(dirname "$agents")"
  ln -sf AGENTS.md "$dir/CLAUDE.md"
done < <(find . -name AGENTS.md -not -path './.git/*' -print0)

# Root .claude -> .agents
if [ -d .agents ]; then
  ln -sfn .agents .claude
fi

echo "link-agents: symlinks refreshed"
