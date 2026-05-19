# Spike — bd → br migration evaluation

> **Status:** open. Decide before adopting `br` anywhere downstream.
> **Owner:** gabriel.
> **Outcome:** a written decision (migrate / stay / hybrid) + recovery plan if
> migrating.

## Why this exists

The user has expressed interest in migrating from `bd` (gastownhall/beads) to
`br` (jeffrey's beads_rust port), motivated primarily by `bv` (graph-aware
triage) integrating more naturally with `br`. This spike collects the criteria
before any commitment is made.

## Decision criteria

Each criterion must be answered with evidence (not vibes) before the decision
is committed.

### 1. Feature parity for what kernl actually uses

kernl is deeply coupled to `bd` today. Inventory before any migration:

- [ ] Which `bd` subcommands does kernl invoke from Go code? (search:
      `grep -r "\"bd\"" orchestrator/ internal/ cmd/`)
- [ ] Which `bd` subcommands does the user invoke interactively? (sample from
      atuin history or `bd memories` log)
- [ ] For each, does `br` have an equivalent subcommand with the same flags
      and behavior?
- [ ] JSONL export format compatibility — kernl parses `.beads/issues.jsonl`
      hermetically in tests. Does `br` produce the same shape?
- [ ] `bd remember` / `bd memories` / `bd forget` — does `br` have this?
      (Six persistent memories are stored via this today; they need a
      migration path.)

### 2. The bv leverage that motivates the migration

- [ ] What specifically does `bv` give us with `br` that we can't get with
      `bd`? Document the concrete workflow improvement, not the abstract one.
- [ ] Can `bv` run against the `bd` JSONL export? If yes, the migration is
      not load-bearing — `bv` works without it.

### 3. Sync model

`bd` uses `refs/dolt/data` on the git remote. `br` uses (TBD — investigate).
The kernl repo sync via `git push`/`git pull` depends on this:

- [ ] What is `br`'s sync model?
- [ ] Are the two compatible on the same remote, or does adopting `br`
      require resetting the remote sync state?

### 4. Hooks / orchestrator integration

The orchestrator's per-stage code calls `bd update --status <next>` to advance
beads. This is wired throughout the workflow state machine:

- [ ] Audit all call sites (`grep -r "bd update" orchestrator/ internal/`).
- [ ] For each, is `br update --status <next>` a drop-in replacement, or are
      there subtle behavioral differences (status enums, error codes,
      transaction semantics)?

### 5. Memory migration

Six `bd remember` entries exist today (see `bd memories`):

- `bd-hygiene-gap-closed-epics-block-downstream-when`
- `kernl-epic-run-architecture-2026-05-17`
- (and four others — list completely before deciding)

For each: does the content survive a `bd → br` migration, or does it need to
be rewritten?

### 6. Cost of being wrong

- [ ] If migration goes badly and we revert: how much work is lost?
- [ ] Can the migration be staged (run both side-by-side for a window, then
      cut over) or is it all-or-nothing?

## Decision matrix

Fill in after evidence is collected:

| Criterion | bd | br | Migration cost | Notes |
|---|---|---|---|---|
| Feature parity | | | | |
| bv leverage | | | | |
| Sync model | | | | |
| Orchestrator integration | | | | |
| Memory migration | | | | |
| Reversibility | | | | |

## Recommended next step

Before any code changes: spend ~2h running the inventory in criterion 1 and
the bv-leverage test in criterion 2. If `bv` works against `bd` JSONL export,
the migration premise weakens significantly and the spike can close as "no
migration; adopt bv on top of bd".

If `bv` requires `br` natively, then continue through the rest of the
criteria with full inventory before any production change.

## Status log

- 2026-05-19 — spike opened. Inventory not yet started.
