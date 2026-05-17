---
name: vc-convert-plan-to-beads
description: 'Deterministically convert an approved plan to a bead graph by running the extraction script. User-confirmed after task review via vc-writing-plans. Also triggers when user says convert plan to beads, plan to beads, or make beads. Zero creative interpretation — script reads ```json bead blocks from the plan markdown, validates, and emits the bd graph JSON.'
user-invocable: true
---

# vc-convert-plan-to-beads

## Overview

This skill is a **mechanical converter**, not a planner. It runs `scripts/extract_beads.py` against an approved plan (where every `### Task` heading is followed by a fenced ```` ```json ```` bead block), validates the graph, and writes the bd graph JSON.

**Key constraint: zero creative interpretation.** The Python script does the extraction — pure regex + `json.loads` + graph validation. The LLM only:
1. Runs the script.
2. Reads the script's exit code and stderr.
3. Reviews the resulting JSON for sanity (does node count match expectations? does the graph shape match the plan's narrative?).
4. Reports back.

**Announce at start:** "Converting the approved plan to beads — running extract_beads.py, zero creative interpretation."

## Why a script

Earlier versions of this skill instructed the LLM to extract structured task data from markdown by hand. Plans run 20-40k tokens; LLM extraction at that scale silently drops tasks, mis-normalizes keys, misses dependencies, and paraphrases descriptions. The script eliminates this class of failure — it either parses 100% correctly or fails loudly with line/column-precise errors. The LLM's job becomes **review**, not **extraction**.

## Input

ONE file: the approved task breakdown markdown produced by `vc-writing-plans`.

- `docs/plans/YYYY-MM-DD-<feature>-tasks.md`

If the task breakdown is missing, STOP. Tell the user the task breakdown must be approved before conversion.

**Rationale:** Do NOT also read `docs/PLAN.md`. The task breakdown is a strict derivative of the plan. Reading both risks duplicate nodes.

## Conversion procedure

### Step 1 — Validate

```bash
python3 .claude/skills/vibe-engineering-mastery/skills/vc-convert-plan-to-beads/scripts/extract_beads.py \
    docs/plans/YYYY-MM-DD-<feature>-tasks.md --validate
```

- Exit code 0 → all bead blocks parse, graph is valid (acyclic, resolvable deps, priority-monotonic, parent-is-epic, unique keys). Proceed to Step 2.
- Exit code 2 → script printed errors on stderr. **STOP.** Report the errors verbatim to the user. The plan must be fixed in `vc-writing-plans` before conversion can continue. Do NOT attempt to hand-fix the plan from this skill.

### Step 2 — Extract

```bash
python3 .claude/skills/vibe-engineering-mastery/skills/vc-convert-plan-to-beads/scripts/extract_beads.py \
    docs/plans/YYYY-MM-DD-<feature>-tasks.md \
    --output beads/YYYY-MM-DD-<feature>-plan.json \
    --commit-message "Create project plan for <feature>"
```

The script:
- Derives `<feature>` from the input filename if `--commit-message` is omitted.
- Creates `beads/` directory if missing.
- Writes the JSON file with stable indentation (2 spaces).
- Reports `N nodes, N edges -> <output>` on stderr.

### Step 3 — Review

Read the generated JSON file. Sanity-check (do NOT re-extract):

1. **Node count matches.** Count `### Task` headings in the source; count `nodes[]` in the output. They must be equal.
2. **Epic structure looks right.** Every task with a `parent_key` should belong to an epic that exists in the same JSON. The script enforces this, but eyeball-confirm the hierarchy matches the plan's narrative.
3. **No silent surprises in metadata.** Spot-check 2-3 tasks: their `description`, `acceptance_criteria`, `files.create/modify/test`, and `estimated_minutes` should match what the plan says.
4. **External ref propagation.** If the plan format does not yet supply `external_ref` in the bead JSON, optionally add it as a post-processing step (see "Optional: external_ref" below).

If anything looks off, STOP and report. Do NOT edit the JSON by hand — fix the plan and re-run.

### Step 4 — Report

Present:

**"Plan transcoded to beads. Summary:**
- [N] tasks mapped to nodes
- [N] dependency edges
- Output: `beads/YYYY-MM-DD-<feature>-plan.json`
- Graph valid: **yes**

The bead graph is ready. Load it with `bd create --graph beads/YYYY-MM-DD-<feature>-plan.json` when you are ready to execute.**"

## Optional: `external_ref`

The script does not auto-populate `external_ref` (provenance link back to the plan file). If you want it on every node, the plan can include `"external_ref": "plan:./docs/plans/YYYY-MM-DD-<feature>-tasks.md"` in each bead JSON block. Alternatively, add a one-time `jq` post-step:

```bash
EXTREF="plan:./docs/plans/YYYY-MM-DD-<feature>-tasks.md"
jq --arg ref "$EXTREF" '.nodes = (.nodes | map(.external_ref //= $ref))' \
   beads/YYYY-MM-DD-<feature>-plan.json > beads/YYYY-MM-DD-<feature>-plan.json.tmp \
&& mv beads/YYYY-MM-DD-<feature>-plan.json.tmp beads/YYYY-MM-DD-<feature>-plan.json
```

## Validation guarantees provided by the script

| Check | Behavior on failure |
|---|---|
| Every `### Task` heading has a ```` ```json ```` block | Hard stop with task ID |
| JSON parses | Hard stop with line/column |
| `key` present, unique, non-empty | Hard stop |
| `title` non-empty, ≤ 500 chars | Hard stop |
| `type` ∈ {bug, feature, task, epic, chore, decision, message, spike, story, milestone} | Hard stop |
| `priority` ∈ integer 0-4 | Hard stop |
| `estimated_minutes` ≥ 0 if set | Hard stop |
| `dependencies` is a list of known keys | Hard stop with offending key |
| `parent` is a known key AND has `type: "epic"` | Hard stop |
| Priority monotonic across `blocks` edges (`from.priority > to.priority`) | Hard stop with offending edge |
| Dependency graph acyclic (DFS) | Hard stop with cycle path |

If the script exits 0, the resulting graph satisfies all of these by construction.

## Edge semantics emitted by the script

- **Dependency edge** (`"type": "blocks"`): from `child` to each entry in `child.dependencies`. Semantics: child cannot start until dependency closes.
- **Parent edge** (`"type": "parent-child"`): from `child` to `child.parent` when `parent` is set. Semantics: child belongs to the epic's hierarchy. Note: this matches the `references/graph-schema.md` edge-type reference (`parent-child` is the correct semantic type for epic membership). The bd CLI consumes this edge type alongside `blocks`.

## Prompt Rules (never violate)

- Do NOT extract task data from the markdown by hand. Run the script.
- Do NOT add, remove, split, or merge tasks. The script transcodes 1:1.
- Do NOT edit the output JSON by hand. If something is wrong, fix the plan and re-run.
- Do NOT infer dependencies the script did not see. If a dependency is missing, it must be added to the plan's bead JSON, not patched into the output.
- Do NOT run `bd create --graph`. That is execution; this skill only converts.

## User Gate

After Step 4 reports success, the user owns whether to execute `bd create --graph`. The skill ends after reporting.

If validation failed at Step 1: report the script's stderr verbatim, name the plan file path that needs fixing, suggest re-running `vc-writing-plans` with the user's chosen fixes, and end cleanly.

(End of file)
