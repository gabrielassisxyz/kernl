# Spike — Adopt compound-engineering pipeline for kernl external dev

> **Status:** decided in principle (see thread 2026-05-18). This spike captures
> the **transition plan** and the **substitution map** so the actual swap
> doesn't get re-decided every time.
> **Owner:** gabriel.

## Why

The current external dev pipeline relies on `vc-brainstorm`, `vc-plan`,
`vc-writing-plans`, `vc-convert-plan-to-beads`, plus `vibe-engineering-mastery`
and `vibe-chaos-to-concept`. These were intended to be temporary scaffolding
until the kernl MVP shipped. The MVP did not ship on the target date and the
scaffolding has hardened into the dev workflow.

Compound-engineering (EveryInc) ships a workflow that:

1. Covers the same surface (strategy → ideate → brainstorm → plan → execute → review).
2. Closes the loop with `/ce-compound` — every shipped unit writes a structured
   learning to `docs/solutions/` with YAML frontmatter compatible with the future
   kernl graph node schema (`Solution`/`Learning`).
3. Uses multi-persona reviewers with deterministic merge/dedup, fully autonomous —
   matches kernl's "zero human gates inside the pipeline" constraint.

This makes it the closer fit for kernl's stated end state, and starting to use
it now bootstraps the knowledge artifacts that the graph substrate (P0.1) will
ingest later.

## Target pipeline (external dev)

```
/ce-strategy                                  # once; maintains docs/STRATEGY.md
                                              # (the existing file is already
                                              # in this shape — no rewrite)

/ce-ideate  OR  /idea-wizard  OR  /dueling-idea-wizards
                                              # optional, high-level ideation;
                                              # pick based on whether you want
                                              # generative, evaluative, or
                                              # adversarial framing

/ce-brainstorm                                # produces docs/brainstorms/<feat>-requirements.md

/ce-plan                                      # produces docs/plans/<date>-<feat>-plan.md

/beads-workflow                               # converts plan → beads (replaces
                                              # vc-convert-plan-to-beads)

kernl epic run <epic-id> --autopilot          # orchestrator executes; no human
                                              # in the loop until done
  └─ implementation_review stage              # uses /ce-code-review multi-persona
  └─ shipment_review stage                    # uses /ce-code-review (integration mode)
                                              #   + /code-review-gemini-swarm-with-ntm
  └─ shipment stage                           # invokes /ce-compound mode:headless
                                              # writing docs/solutions/<categoria>/*.md

[PR opened on GitHub — human reviews/approves/merges]

post-merge hook (webhook or manual marker)    # appends PR review comments to
                                              # the same compound doc — primary
                                              # capture channel for human judgment
```

## Human gates

Exactly two, both **outside** the orchestrator pipeline:

1. **Plan approval** — after `/ce-plan`, before `/beads-workflow`. Optional in
   theory; recommended in practice for non-trivial features.
2. **PR review** — after the orchestrator opens the PR. The user reviews and
   merges (or rejects). The post-merge hook then captures those comments.

No `plan_review`/`implementation_review`/`shipment_review` stage waits on the
human. They run autonomously with multi-persona reviewers.

## Substitution map

| Replaced | By | Notes |
|---|---|---|
| `vc-brainstorm` | `/ce-brainstorm` | Output: `docs/brainstorms/*.md`. CE's brainstorm produces a requirements doc that `/ce-plan` consumes directly. |
| `vc-plan`, `vc-writing-plans` | `/ce-plan` | Output: `docs/plans/<date>-<slug>-plan.md`. CE's plan output is portable across executors. |
| `vc-convert-plan-to-beads` | `/beads-workflow` | jeffrey-skill that does the same conversion. **Verify** it handles kernl's bead-mapping metadata before full cutover. |
| `vibe-engineering-mastery` | the CE pipeline as a whole | Same chain (strategy → reviews → planning); CE just has the compounding loop too. |
| `vibe-chaos-to-concept` | `/ce-ideate` or `/idea-wizard` | Use whichever feels right for the input shape. |

## Things that stay

- **`bd`** as the issue tracker beneath `/beads-workflow`. (`bd → br` is a
  separate spike — see `2026-05-19-bd-to-br-migration.md`.)
- **`docs/STRATEGY.md`** — already in CE-compatible form.
- **`docs/VISION.md`** — not a CE artifact; stays as kernl's vision doc.
- **`docs/suggested-vision-projects.md`** — kernl-specific decomposition;
  stays.

## Transition plan

Staged so in-flight work isn't disrupted:

1. **Now (next bead/feature):** use the CE pipeline end-to-end on a single
   new feature. Validate that `/beads-workflow` handles the plan output and
   that the orchestrator picks up the resulting beads without changes.
2. **In-flight `vc-*` work:** finish in-flight features on the old pipeline
   (no mid-stream re-tooling). Do not start new `vc-*` cycles.
3. **After 3-5 features ship via CE:** mark `vc-*` skills as deprecated for
   kernl dev. Move them to a "legacy" section in personal notes (don't delete
   — they may be useful for other projects).
4. **Pre-P0.1:** verify that the accumulated `docs/solutions/*` files have a
   migration path to `Solution`/`Learning` graph nodes. If frontmatter
   coverage is good, the migration is a simple script.

## Open questions

- [ ] Does `/beads-workflow` accept the metadata that kernl's bead JSON
      schema needs (epic linking, stage profile, etc.)? If not, what's the
      smallest adapter?
- [ ] How is the post-merge GitHub hook wired? (Webhook receiver in kernl
      itself, GH Actions step, or manual `kernl pr-merged <pr-num>` command?)
      Decide before the first feature ships via CE.
- [ ] `/ce-compound` writes to `docs/solutions/<categoria>/*.md`. The
      categoria list is fixed by CE's schema. Does that taxonomy match what
      kernl wants long-term, or do we want a kernl-specific override?

## Status log

- 2026-05-19 — spike opened. Decision recorded; transition not yet started.
