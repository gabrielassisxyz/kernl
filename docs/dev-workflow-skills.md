# Dev Workflow Skills — Periodic Hygiene Rotation

> **Status:** manual today. These are the routines that the suggested
> sub-project **P3.8 — Scheduled maintenance workflows** will eventually
> automate (see `docs/suggested-vision-projects.md`). Until P3.8 lands, run
> these manually at the suggested triggers below.

## How to use this list

Each entry has a **trigger** (when to run), a **command** (what to invoke), and a
**why** (what failure mode it catches). The triggers are deliberate, not
arbitrary cadence: prefer "before milestone" over "every Friday" — the
calendar-pegged ones are fallbacks for when no natural trigger exists.

The columns map 1:1 to what P3.8 needs to encode: each row here is a candidate
shape (frequency, scope, severity threshold) for the cron workflow engine.

---

## Pre-milestone / pre-release gates

Run these **before declaring any milestone or MVP done**. They catch the
specific failure modes that swarm-driven development produces.

| Skill | Trigger | Why |
|---|---|---|
| `/beads-compliance-and-completion-verification` | Before any milestone marked "done"; or after a swarm closes ≥10 beads in a row | Audits closed beads against acceptance criteria. The skill itself notes "30–60% of closed beads in long swarms were never actually implemented." kernl's bd-status-drift memory confirms this is a real failure mode here. |
| `/mock-code-finder` | Same trigger as above | Catches stubs/mocks/placeholders that silently passed review. Pairs with the compliance audit. |
| `/reality-check-for-project` | Weekly; or whenever the user says "where are we", "is the swarm still pointed at the right thing" | Articulates current implementation state vs. `docs/VISION.md`/`docs/STRATEGY.md`. Catches drift between bead-count progress and product-vision progress. |
| `/ubs` (Ultimate Bug Scanner) | Before every PR opened on `master` | Broad bug/quality scan. Cheap relative to a real review. |

## Periodic codebase audits (rotational)

Cycle through these; doing all at once is heavy.

| Skill | Trigger | Why |
|---|---|---|
| `/codebase-audit` (mode: security) | Monthly; before any opensource release | Security-domain audit. |
| `/codebase-audit` (mode: performance) | When a stage's wall-clock noticeably regresses, or quarterly | Catches perf regressions before they become customer-visible. |
| `/codebase-audit` (mode: api) | When changing public CLI surface or REST handlers | Contract drift detection. |
| `/codebase-audit` (mode: docs) | When VISION/STRATEGY/PLAN changes substantially | Catches doc drift against implementation. |
| `/codebase-audit` (mode: ux) / `/ux-audit` | Once GUI exists; after every meaningful UI change | Defers until P2.6. |
| `/profiling-software-performance` | Triggered by perf-audit findings | Identifies hot paths before optimization. |
| `/extreme-software-optimization` | After profiling identifies a hotspot the user cares about | Profile-driven optimization with proof-of-improvement. |
| `/simplify-and-refactor-code-isomorphically` | Quarterly; or after a feature spike that left scaffolding behind | Shrinks code without behavior change. |
| `/codebase-pattern-extraction` | Before opensource; after major architectural shift | Surfaces reusable patterns into explicit docs. |
| `/codebase-report` | Before any new contributor onboards | Generates the architecture doc for hand-off. Combines with `/codebase-archaeology` for "why is this here". |

## Git/repo hygiene

| Skill | Trigger | Why |
|---|---|---|
| `/git-worktree-branch-rationalization` | When `git worktree list` exceeds ~20 entries; after any swarm leaves debris | kernl uses worktrees aggressively per epic — debris accumulates. |
| `/git-stash-janitor` | When `git stash list` exceeds ~10 entries; or quarterly | Same family. |
| `/git-repo-janitor` | When the repo root accumulates orphan `.md`/`.json`/`.db` files | Catches the artifacts that bd/skill swarms drop and never clean up. |

## Dependency / library maintenance

| Skill | Trigger | Why |
|---|---|---|
| `/library-updater` | Monthly; or when a CVE drops on a dep | Bumps `go.mod`. |
| `/research-software` | Before deciding to add or replace any external dep | Already used informally; promote to mandatory before lib decisions. |

## Release / shipment

| Skill | Trigger | Why |
|---|---|---|
| `/release-preparations` | When cutting a release | Runs the full pre-release gate (tests, version, cross-platform build, GH release with checksums). Lives inside the `shipment` stage long-term. |
| `/changelog-md-workmanship` | Quarterly; or before any release | Rebuilds `CHANGELOG.md` from git/tags. |
| `/readme-writing` | Before opensource; after major feature waves | Keeps README from rotting against the actual product. |
| `/documentation-website-for-software-project` | Pre-opensource | Nextra docs site. Deferred until kernl has users beyond you. |

## DevEx / agent-facing surface

| Skill | Trigger | Why |
|---|---|---|
| `/agent-ergonomics-and-intuitiveness-maximization-for-cli-tools` | Before stabilizing v1 CLI surface; after any new top-level command | The kernl CLI is consumed primarily by agents. Surface ergonomics matter before — not after — the contract stabilizes. |
| `/world-class-doctor-mode-for-cli-tools` | Anytime kernl's `doctor` subcommand shows gaps vs. the gold-standard pattern (capabilities reflection, robot-docs, scoring artifact) | Convergence target for our existing `doctor`. |

## Debugging / when something is wrong

Not periodic — invoked on demand, listed so the trigger is remembered.

| Skill | Trigger |
|---|---|
| `/gdb-for-debugging` | When `kernl epic run <bead-id>` hangs and the cause is unclear. Attaches to the live process; reads /proc; gets a real backtrace instead of guesswork. |
| `/deadlock-finder-and-fixer` | When a stage flakes intermittently or two beads silently block each other. kernl is goroutine-per-session + single-flight on MergeManager — high prior. |
| `/multi-pass-bug-hunting` | When the bug survived a first fix and you're going in for round two. |
| `/repeatedly-apply-skill` | When one pass of a skill produced "OK" output and you want to deepen iteratively (yegge-loop equivalent). |
| `/system-performance-remediation` | When kernl itself runs slow on the host (RAM, CPU, IO contention with the swarm). |

---

## Bridge to P3.8

Each row above becomes a candidate cron shape in **P3.8**. When P3.8 is
brainstormed, the conversion is:

```
trigger column   →  shape's cron expression or event-binding
command column   →  shape's stage list
why column       →  shape's "criteria for producing remediation beads"
```

Keep this file accurate as the manual reality so P3.8's brainstorm has a
specification to start from instead of a blank page.
