---
name: vc-convert-plan-to-beads
description: 'Deterministically convert an approved plan to a bead graph. User-confirmed after task review via vc-writing-plans. Also triggers when user says convert plan to beads, plan to beads, or make beads. Zero creative interpretation — pure mechanical transcoding from markdown Bead Mapping metadata to JSON graph.'
user-invocable: true
---

# vc-convert-plan-to-beads

## Overview

This skill is a **mechanical converter**, not a planner. It reads an approved plan (and its task breakdown) where every task already contains a structured `**Bead Mapping:**` block, and produces a bead graph JSON file using the exact `bd create --graph` schema.

**Key constraint: zero creative interpretation.** Every task maps 1:1 to a node. No tasks are added, removed, split, merged, reworded, or reinterpreted. If the source plan is approved, the output beads are correct by construction.

**Announce at start:** "Converting the approved plan to beads — mechanical transcoding, zero creative interpretation."

## Input

Read ONE file only — the GLM-5.1 task breakdown which contains the final, refined tasks:

- `docs/plans/YYYY-MM-DD-<feature>-tasks.md` — the approved task breakdown (contains `**Bead Mapping:**` blocks)

If the task breakdown is missing, STOP. Tell the user the task breakdown must be approved before conversion.

**Rationale:** Do NOT also read `docs/PLAN.md`. The task breakdown is a strict derivative of the plan. Reading both risks duplicate nodes.

## Conversion Rules

### 1. Node Creation (1:1 per task)

For every `### Task N: [Title]` heading in the source document, emit exactly one node.

| Source Field | Bead Field | Transformation |
|--------------|------------|----------------|
| Heading title | `title` | Use the text after `### Task N:` verbatim |
| `Type:` | `type` | Map directly: `epic`→`epic`, `story`→`story`, `task`→`task`, `chore`→`chore`, `spike`→`spike` |
| `Priority:` | `priority` | Map directly as integer: `0`→0, `1`→1, `2`→2, `3`→3, `4`→4 |
| `Estimated Minutes:` | `estimated_minutes` | Map directly as integer |
| `Status:` | `status` | Map directly: `open`→`open` |
| `Parent:` (non-`none`) | `parent_key` | Set to the key of the referenced epic node |
| `Description / Steps:` | `description` | Concatenate all step text and code blocks (preserve markdown formatting) |
| `Files:` | `metadata.files` | As JSON array under `metadata`: `{"create": [...], "modify": [...], "test": [...]}` |
| `Acceptance Criteria:` | `acceptance_criteria` | Concatenate criteria as bullet list (preserve markdown) |

**Node key format:** Normalize each `### Task N:` heading ID to a URL-safe key. The key prefix is ALWAYS `task-` regardless of the node's actual `type` (the `type` field distinguishes epics from tasks; keys are just identifiers).

1. Strip the `### Task` prefix from the heading
2. Keep the remaining identifier (e.g., `1`, `1.1`, `A`, `setup`)
3. Lowercase
4. Replace spaces with hyphens
5. Strip ALL non-alphanumeric characters (including dots) except hyphens
6. Prepend `task-`
7. Examples: `Task 1` → `"task-1"`, `Task 1.2` → `"task-1-2"`, `Task Setup DB` → `"task-setup-db"`, `Task A` → `"task-a"`

### 2. Edge Creation (from Dependencies)

For every task that declares `Dependencies:` with one or more task references:

- Normalize each dependency reference to a key using the same key-normalization rules as node keys (strip `Task` prefix, lowercase, strip dots/whitespace, hyphenate, prepend `task-`).
- Create an edge from the dependent task's key to each normalized dependency key.
- Edge type is **always** `blocks`.
- Order: the dependent node is `from_key`, the dependency target is `to_key`.
- Example: `Task 3` depends on `Task 1` → edge `{ "from_key": "task-3", "to_key": "task-1", "type": "blocks" }`

If `Dependencies:` is `none`, no edges are created for that task.

**Important:** If a dependency reference cannot be normalized to a key that matches any node, this is a validation failure (unresolvable dependency).

### 3. Epic Relationship Edges

For every task that declares a `Parent:` (non-`none`):

- Normalize the parent reference to a key using the same key-normalization rules.
- Set `parent_key` on the node to the normalized parent's key.
- Create a dependency edge from the child task's key to the normalized parent epic's key.
- Edge type is **always** `blocks`.

**Important:** The referenced parent MUST have `type: "epic"`. If not, this is a validation failure (invalid epic relationship).

### 4. Graph Validation (Mandatory)

After building the full graph, run these validations in order. **Any failure is a hard STOP.**

1. **Acyclicity check:** The dependency graph must contain no cycles. Run a DFS or use Tarjan's algorithm. If a cycle is found, report the cycle path and STOP.
2. **Resolvable dependencies:** Every dependency reference in `edges[].to_key` must map to an existing node in `nodes[]`. If unresolved, report the missing key and STOP.
3. **Completeness:** Every `Task` heading in the source MUST have a corresponding node in the output. Count nodes and count tasks; they must match 1:1.
4. **Schema validity:** Every node MUST have `title`, `type`, `priority`, and `status`. No node may have an invalid `priority` (must be 0-4) or invalid `type`.
5. **Priority consistency with dependency order:** For every edge, `from_key`'s priority MUST be strictly greater than `to_key`'s priority. If not, report the edge and STOP.
6. **Valid epic relationships:** Every task with a `parent_key` must point to a node with `type: "epic"`.
7. **No duplicate keys:** All `nodes[].key` must be unique.

If all validations pass, proceed to output.

## Output

### Output File Path

```
beads/YYYY-MM-DD-<feature>-plan.json
```

- The `YYYY-MM-DD` and `<feature>` segments are derived from the input task breakdown filename.
- **Directory creation:** Ensure `beads/` directory exists before writing. If it does not exist, create it first (e.g., `mkdir -p beads/` if needed).
- The `commit_message` field in the JSON should read: `"Create project plan for <feature>"`.

### JSON Schema

```json
{
  "commit_message": "Create project plan for <feature>",
  "nodes": [
    {
      "key": "task-1",
      "title": "Set up project scaffolding",
      "type": "task",
      "priority": 0,
      "status": "open",
      "estimated_minutes": 5,
      "external_ref": "plan:./docs/plans/YYYY-MM-DD-feature-tasks.md",
      "description": "...",
      "acceptance_criteria": "...",
      "metadata": {
        "files": {
          "create": ["src/main.py"],
          "modify": [],
          "test": ["tests/test_main.py"]
        }
      }
    }
  ],
  "edges": [
    {
      "from_key": "task-3",
      "to_key": "task-1",
      "type": "blocks"
    }
  ]
}
```

### Constraints on Fields

- `title`: required, max 500 characters. If a title exceeds 500 chars, report as a validation issue.
- `external_ref`: optional. Set to `"plan:./docs/plans/YYYY-MM-DD-<feature>-tasks.md"` using the source filename verbatim for provenance.
- `priority`: required, integer 0-4
- `type`: required, one of `bug`, `feature`, `task`, `epic`, `chore`, `decision`, `message`, `spike`, `story`, `milestone`
- `status`: required, "open"
- `estimated_minutes`: optional, non-negative integer
- `description`: optional, markdown allowed
- `acceptance_criteria`: optional, markdown allowed
- `metadata`: optional, well-formed JSON object

## Output Delivery

After writing the validated JSON file:

1. Report the file path: `beads/YYYY-MM-DD-<feature>-plan.json`
2. Report validation summary: [N] nodes, [N] edges, graph valid.
3. STOP. Do NOT run `bd create --graph`. That is execution and this skill only converts.

## Prompt Rules (never violate)

- Do NOT add tasks that are not in the source plan.
- Do NOT remove tasks from the source plan.
- Do NOT split or merge tasks.
- Do NOT reword titles, descriptions, or acceptance criteria.
- Do NOT infer dependencies that are not explicitly declared in `Dependencies:`.
- Do NOT change priorities, types, or parent relationships.
- Do NOT estimate times — use `estimated_minutes` from the source verbatim.
- Do NOT add "cleanup", "review", or "deploy" beads unless they exist as tasks in the source.

## User Gate

After conversion and validation, present:

**"Plan transcoded to beads. Summary:**
- [N] tasks mapped to nodes
- [N] dependency edges
- Output: `beads/YYYY-MM-DD-<feature>-plan.json`

If validation passed:
- **Graph valid:** yes
- The bead graph is ready. Load it with `bd create --graph beads/YYYY-MM-DD-<feature>-plan.json` when you are ready to execute.

If validation failed:
- **Graph valid:** no
- STOP. Report the specific failures and end cleanly. Do not write the JSON file. The user must fix the plan and re-run conversion.

(End of file)
