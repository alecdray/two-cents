#!/usr/bin/env bash
#
# check-docs-provider.sh — mechanically assert the docs/config describe Plaid as the
# current bank provider, and that every remaining "Teller" mention is explicitly
# historical/superseded context (not a claim that Teller is the current provider).
#
# Run from the repo root:  ./scripts/check-docs-provider.sh
# Optional section arg:    ./scripts/check-docs-provider.sh [env|prd|teller|plaid]
#   (no arg = run every section)
#
# Exit 0 = pass, non-zero = fail.

set -u

# Resolve repo root as the parent of this script's dir, so it runs from anywhere.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT" || exit 2

fail=0
note() { printf '  %s\n' "$*"; }
ok()   { printf 'PASS  %s\n' "$*"; }
bad()  { printf 'FAIL  %s\n' "$*"; fail=1; }

# The set of doc/config files this pivot governs. Dockerfile is included
# because its comments document the deployment artifacts (e.g. what the mounted
# volume holds), which is exactly where a stale provider claim can hide.
DOC_GLOBS=(docs README.md CONTEXT.md .env.template docker-compose.example.yml .claude/CLAUDE.md Dockerfile)

# ---- section: .env.template ------------------------------------------------
check_env() {
  printf '== .env.template ==\n'
  if grep -q 'PLAID_CLIENT_ID' .env.template; then ok ".env.template has PLAID_CLIENT_ID"; else bad ".env.template missing PLAID_CLIENT_ID"; fi
  if grep -q 'PLAID_SECRET'    .env.template; then ok ".env.template has PLAID_SECRET";    else bad ".env.template missing PLAID_SECRET";    fi
  if grep -q 'TELLER_'         .env.template; then bad ".env.template still contains a TELLER_ token"; else ok ".env.template has no TELLER_ token"; fi
}

# ---- section: docs/prd.md module name --------------------------------------
check_prd() {
  printf '== docs/prd.md ==\n'
  if grep -Eq '`plaid`' docs/prd.md; then ok "docs/prd.md references a \`plaid\` module"; else bad "docs/prd.md does not reference a \`plaid\` module"; fi
  if grep -Eq '`teller`' docs/prd.md; then bad "docs/prd.md still references a \`teller\` module"; else ok "docs/prd.md references no \`teller\` module"; fi
}

# ---- section: Plaid terminology present where expected ---------------------
check_plaid() {
  printf '== Plaid terminology ==\n'
  assert_present() { # <term> <file>
    if grep -Fq "$1" "$2"; then ok "$2 mentions \"$1\""; else bad "$2 missing \"$1\""; fi
  }
  assert_present 'access_token'              docs/adr/0002-bankprovider-abstraction.md
  assert_present 'personal_finance_category' docs/adr/0003-two-layer-transfer-detection.md
  assert_present 'personal_finance_category' docs/domain/README.md
  assert_present 'Plaid Link'                docs/prd.md
  assert_present 'public_token'              docs/prd.md
  assert_present '/transactions/sync'        docs/domain/README.md
  if grep -Eqi 'cursor' docs/domain/README.md; then ok "docs/domain/README.md describes cursor-based sync"; else bad "docs/domain/README.md missing cursor reference"; fi
}

# ---- section: Teller allowlist --------------------------------------------
# Every surviving (case-insensitive) "teller" occurrence must sit in an explicitly
# historical/superseded context. The allowlist below is a set of regexes; a line
# matching ANY of them is acceptable history. Anything else fails the check.
check_teller() {
  printf '== Teller mentions (must all be historical) ==\n'
  # Allowed-history markers (case-insensitive).
  local allow='(was Teller|Teller→Plaid|Teller-vs-Plaid|switched to Plaid|switched off Teller|then switched|originally chose|chose Teller|we chose Teller|under Teller|Teller had to go|Teller closed|Teller'"'"'s flat|replacing the Teller|replacing Teller|Plaid replacing Teller|chosen over a hosted|rejected|superseded|history)'

  # Collect every teller hit across the governed set, with file:line.
  local hits
  hits="$(grep -rniE 'teller' "${DOC_GLOBS[@]}" 2>/dev/null)"

  if [ -z "$hits" ]; then
    ok "no Teller mentions at all"
    return
  fi

  local any_bad=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    if printf '%s' "$line" | grep -Eiq "$allow"; then
      note "[history ] $line"
    else
      note "[ORPHAN  ] $line"
      any_bad=1
    fi
  done <<< "$hits"

  if [ "$any_bad" -eq 0 ]; then
    ok "every Teller mention is explicitly historical/superseded"
  else
    bad "found Teller mention(s) outside the historical allowlist (see [ORPHAN] above)"
  fi
}

section="${1:-all}"
case "$section" in
  env)    check_env ;;
  prd)    check_prd ;;
  plaid)  check_plaid ;;
  teller) check_teller ;;
  all)    check_env; check_prd; check_plaid; check_teller ;;
  *) echo "unknown section: $section (use: env|prd|plaid|teller|all)"; exit 2 ;;
esac

printf '\n'
if [ "$fail" -eq 0 ]; then
  echo "RESULT: PASS"
else
  echo "RESULT: FAIL"
fi
exit "$fail"
