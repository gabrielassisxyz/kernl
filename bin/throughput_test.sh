#!/usr/bin/env bash
# Unit tests for the attribution logic in bin/throughput: the ledger readers
# (ledger_commit_shas, ledger_epic_branches), the classifiers
# (classify_by_sha_prefix, classify_by_branch), and cmd_report's split output,
# all exercised against fixture files - never the operator's real ~/.kernl,
# never a real git/gh call, and no write outside a temp directory.
#
# bin/throughput has no prior test coverage or test framework to follow, so
# this is the smallest thing that gives real coverage: plain bash assertions,
# every check run in its own subshell so `bin/throughput`'s `set -euo
# pipefail` (which sourcing inherits into the subshell only) can never abort
# the test runner itself.
#
#   bin/throughput_test.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
THROUGHPUT_BIN="$REPO_ROOT/bin/throughput"

fail=0
checked=0

ok()   { checked=$((checked + 1)); }
bad()  { checked=$((checked + 1)); fail=1; printf 'FAIL: %s\n' "$1" >&2; }

assert_eq() {
    local expected="$1" actual="$2" msg="$3"
    if [ "$expected" = "$actual" ]; then
        ok
    else
        bad "$msg - expected [$expected], got [$actual]"
    fi
}

assert_contains() {
    local haystack="$1" needle="$2" msg="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        ok
    else
        bad "$msg - expected to find [$needle]"
    fi
}

# Runs one bin/throughput function in an isolated subshell: sourcing sets
# `set -euo pipefail` and defines every function, then "$@" invokes the one
# under test. The subshell means that strict mode - and any global the
# sourced file sets - never leaks into this test runner's own shell.
run_fn() {
    ( source "$THROUGHPUT_BIN"; "$@" )
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# --- fixture ledger: two epics, each with an attempt that recorded a
# short (git rev-parse --short) commit SHA, plus one attempt with no commit
# (a spawn failure never produces one - see StageAttemptRecord.CommitSHA)
# to prove that a null/empty commitSHA is skipped rather than matching
# every commit.
mkdir -p "$work/state/run/epic-a" "$work/state/run/epic-b"
cat > "$work/state/run/epic-a/attempts.jsonl" <<'EOF'
{"epicId":"epic-a","beadId":"epic-a-bead1","stage":"implementation","commitSHA":"abc1234"}
{"epicId":"epic-a","beadId":"epic-a-bead1","stage":"implementation_review","commitSHA":"abc1234"}
{"epicId":"epic-a","beadId":"epic-a-bead2","stage":"implementation","commitSHA":null}
EOF
cat > "$work/state/run/epic-b/attempts.jsonl" <<'EOF'
{"epicId":"epic-b","beadId":"epic-b-bead1","stage":"integration","commitSHA":"def5678"}
EOF

# === ledger_commit_shas ===
out="$(run_fn ledger_commit_shas "$work/state/run")"
assert_eq "$(printf 'abc1234\ndef5678')" "$out" \
    "ledger_commit_shas returns the distinct non-null commit SHAs across every epic"

out="$(run_fn ledger_commit_shas "$work/state/does-not-exist")"
assert_eq "" "$out" "ledger_commit_shas on a missing run dir yields nothing, not an error"

# === ledger_epic_branches ===
out="$(run_fn ledger_epic_branches "$work/state/run")"
assert_eq "$(printf 'feat/epic-a\nfeat/epic-b')" "$out" \
    "ledger_epic_branches emits feat/<epicID> for every epic with a ledger"

# === classify_by_sha_prefix: prefix match, not equality ===
printf 'abc1234\ndef5678\n' > "$work/shas.txt"
cat > "$work/commits_body.csv" <<'EOF'
repoA,abc1234fullsha000000000000000000000000,2026-07-01T10:00:00-03:00,3,10,2
repoA,def5678fullsha000000000000000000000000,2026-07-02T11:00:00-03:00,1,5,1
repoA,ffffffffffffffffffffffffffffffffffffff,2026-07-03T09:00:00-03:00,2,20,0
EOF
out="$(run_fn classify_by_sha_prefix "$work/commits_body.csv" 2 "$work/shas.txt")"
assert_eq "$(printf \
    'repoA,abc1234fullsha000000000000000000000000,2026-07-01T10:00:00-03:00,3,10,2,1\nrepoA,def5678fullsha000000000000000000000000,2026-07-02T11:00:00-03:00,1,5,1,1\nrepoA,ffffffffffffffffffffffffffffffffffffff,2026-07-03T09:00:00-03:00,2,20,0,0')" \
    "$out" \
    "classify_by_sha_prefix marks the two ledger-prefixed commits 1 and the unmatched one 0"

# A ledger SHA is never treated as equal to a commit whose full SHA merely
# ENDS the same way - guards against a classifier that accidentally matches
# on suffix or substring instead of prefix. "234" is the tail of
# "abc1234...", never its head, so it must match nothing.
printf '234\n' > "$work/suffix_only.txt"
out="$(run_fn classify_by_sha_prefix "$work/commits_body.csv" 2 "$work/suffix_only.txt")"
assert_contains "$out" "repoA,abc1234fullsha000000000000000000000000,2026-07-01T10:00:00-03:00,3,10,2,0" \
    "classify_by_sha_prefix leaves the abc1234... row unmatched against the '234' fragment"
assert_contains "$out" "repoA,def5678fullsha000000000000000000000000,2026-07-02T11:00:00-03:00,1,5,1,0" \
    "classify_by_sha_prefix leaves the def5678... row unmatched against the '234' fragment"

# === classify_by_branch: exact match, unrelated branch untouched ===
printf 'feat/epic-a\nfeat/epic-b\n' > "$work/branches.txt"
cat > "$work/prs_body.csv" <<'EOF'
repoA,10,MERGED,2026-07-01T09:00:00Z,2026-07-01T12:00:00Z,2026-07-01T12:00:00Z,15,3,2,3,feat/epic-a
repoA,11,OPEN,2026-07-02T09:00:00Z,,,8,1,1,,fix/manual-thing
repoA,12,MERGED,2026-07-03T09:00:00Z,2026-07-03T15:00:00Z,2026-07-03T15:00:00Z,4,0,1,6,feat/epic-b
EOF
out="$(run_fn classify_by_branch "$work/prs_body.csv" 11 "$work/branches.txt")"
assert_contains "$out" "repoA,10,MERGED,2026-07-01T09:00:00Z,2026-07-01T12:00:00Z,2026-07-01T12:00:00Z,15,3,2,3,feat/epic-a,1" \
    "classify_by_branch marks a PR opened from a ledger epic branch as orchestrated"
assert_contains "$out" "repoA,11,OPEN,2026-07-02T09:00:00Z,,,8,1,1,,fix/manual-thing,0" \
    "classify_by_branch marks a PR from an unrelated branch as hand-produced"

# === cmd_report: end-to-end split, --by repo (no date/timezone dependence) ===
mkdir -p "$work/data"
{
    echo "repo,sha,author_date,files_changed,insertions,deletions"
    cat "$work/commits_body.csv"
} > "$work/data/commits.csv"
{
    echo "repo,number,state,created_at,merged_at,closed_at,additions,deletions,changed_files,lead_time_hours,head_ref_name"
    cat "$work/prs_body.csv"
} > "$work/data/prs.csv"

report_out="$(
    (
        source "$THROUGHPUT_BIN"
        COMMITS_CSV="$work/data/commits.csv"
        PRS_CSV="$work/data/prs.csv"
        LEDGER_RUN_DIR="$work/state/run"
        cmd_report --by repo
    ) 2>&1
)"

assert_contains "$report_out" "CLASSIFICATION CAVEAT" \
    "cmd_report names the caveat instead of relying on print order"
assert_contains "$report_out" "not AI-vs-human" \
    "cmd_report's caveat states this is an orchestrated-vs-supervised split, not AI-vs-human"
assert_contains "$report_out" "squash-merged" \
    "cmd_report's caveat names the squash-merge SHA undercount, not just the orchestrated-vs-supervised framing"
assert_contains "$report_out" "COMMITS_ORCH systematically UNDERCOUNTS" \
    "cmd_report's caveat says which column undercounts and that PRS_*_ORCH does not"

# 3 commits total, 2 matched by SHA prefix (abc1234, def5678); repoA row.
repo_line="$(printf '%s\n' "$report_out" | grep '^repoA' | head -n1)"
assert_contains "$repo_line" "repoA" "cmd_report --by repo emits a repoA row"
read -r bucket commits commits_orch _files _ins _dels opened opened_orch merged merged_orch _avg <<<"$repo_line"
assert_eq "repoA" "$bucket" "repo bucket key"
assert_eq "3" "$commits" "3 commits mined for repoA"
assert_eq "2" "$commits_orch" "2 of 3 commits matched the ledger by SHA prefix"
assert_eq "3" "$opened" "3 PRs opened for repoA"
assert_eq "2" "$opened_orch" "2 of 3 PRs matched the ledger by head branch"
assert_eq "2" "$merged" "2 PRs merged for repoA (the OPEN one is excluded)"
assert_eq "2" "$merged_orch" "both merged PRs matched the ledger"

assert_contains "$report_out" "classified 2 of 3 commits as orchestrator-produced" \
    "cmd_report prints the whole-run commit classification total"
assert_contains "$report_out" "classified 2 of 3 PRs as orchestrator-produced" \
    "cmd_report prints the whole-run PR classification total"

# === cmd_report with no ledger at all: every row is hand-produced by
# definition (the "before the ledger existed" period) ===
report_out_no_ledger="$(
    (
        source "$THROUGHPUT_BIN"
        COMMITS_CSV="$work/data/commits.csv"
        PRS_CSV="$work/data/prs.csv"
        LEDGER_RUN_DIR="$work/state/no-such-run-dir"
        cmd_report --by repo
    ) 2>&1
)"
assert_contains "$report_out_no_ledger" "classified 0 of 3 commits as orchestrator-produced" \
    "with no ledger, every commit classifies as hand-produced"
assert_contains "$report_out_no_ledger" "classified 0 of 3 PRs as orchestrator-produced" \
    "with no ledger, every PR classifies as hand-produced"

echo
if [ "$fail" -eq 0 ]; then
    printf 'throughput_test.sh: %d checks passed\n' "$checked"
else
    printf 'throughput_test.sh: FAILURES among %d checks\n' "$checked" >&2
fi
exit "$fail"
