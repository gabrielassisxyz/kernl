# Bead Graph JSON Schema Reference

This document defines the exact JSON schema used by `bd create --graph`.

## Top-Level Object

```json
{
  "commit_message": "string",
  "nodes": [Node],
  "edges": [Edge]
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `commit_message` | string | Yes | Dolt commit message for the batch creation |
| `nodes` | array[Node] | Yes | All beads to create |
| `edges` | array[Edge] | Yes | Dependency relationships between beads |

## Node Object

```json
{
  "key": "string",
  "title": "string",
  "type": "string",
  "priority": 0,
  "status": "open",
  "description": "string",
  "acceptance_criteria": "string",
  "estimated_minutes": 0,
  "external_ref": "string",
  "metadata": {},
  "parent_key": "string"
}
```

### Fields

| Field | Type | Required | Constraints |
|-------|------|----------|-------------|
| `key` | string | Yes | Unique within the graph. Used in edges. |
| `title` | string | Yes | Max 500 characters |
| `type` | string | Yes | One of: `bug`, `feature`, `task`, `epic`, `chore`, `decision`, `message`, `spike`, `story`, `milestone` |
| `priority` | integer | Yes | 0-4 (0=critical, 4=backlog) |
| `status` | string | Yes | `open` for new beads |
| `description` | string | No | Markdown allowed. Sourced from task steps. |
| `acceptance_criteria` | string | No | Markdown allowed. Sourced from task acceptance criteria. |
| `estimated_minutes` | integer | No | Non-negative if set |
| `external_ref` | string | No | Optional cross-reference to source document (e.g., `"plan:./docs/plans/YYYY-MM-DD-feature-tasks.md"`) |
| `metadata` | object | No | Arbitrary well-formed JSON. Use for file paths, commands, etc. |
| `parent_key` | string | No | Must reference a Node with `type: "epic"` |

## Edge Object

```json
{
  "from_key": "string",
  "to_key": "string",
  "type": "blocks"
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from_key` | string | Yes | The dependent task (the one that is blocked) |
| `to_key` | string | Yes | The dependency target (the task that must complete first) |
| `type` | string | Yes | Relationship type. Default to `blocks` for standard blocking dependencies. |

### Edge Type Reference

| Type | Semantics | When to use |
|------|-----------|-------------|
| `blocks` | `from` cannot start until `to` closes | Default for all task dependencies |
| `parent-child` | Child is part of epic hierarchy | Used for parent-child epic relationships |
| `conditional-blocks` | `from` runs only if `to` fails | Error-handling paths |
| `waits-for` | Gate waiting for dynamic children | Fanout patterns |
| `related` | Informational link | Non-blocking cross-references |
| `validates` | Test or verification link | Test beads that validate implementation beads |

## Validation Rules Summary

1. All `nodes[].key` unique
2. All `edges[].to_key` must exist in `nodes`
3. Graph must be acyclic (no dependency cycles)
4. For every edge, `from_key` priority > `to_key` priority
5. Every `parent_key` must point to a node with `type: "epic"`
6. All required fields present on every node
7. `estimated_minutes` >= 0 if set
8. `metadata` must be valid JSON if set

## Example Complete Graph

```json
{
  "commit_message": "Create auth feature plan",
  "nodes": [
    {
      "key": "task-auth-system",
      "title": "Authentication System",
      "type": "epic",
      "priority": 0,
      "status": "open",
      "description": "Implement OAuth 2.0 authentication for the API gateway."
    },
    {
      "key": "task-1",
      "title": "Design auth schema",
      "type": "task",
      "priority": 0,
      "status": "open",
      "external_ref": "plan:./docs/plans/2024-01-15-auth-tasks.md",
      "parent_key": "task-auth-system",
      "estimated_minutes": 15,
      "description": "...",
      "acceptance_criteria": "..."
    },
    {
      "key": "task-2",
      "title": "Implement OAuth endpoints",
      "type": "task",
      "priority": 1,
      "status": "open",
      "external_ref": "plan:./docs/plans/2024-01-15-auth-tasks.md",
      "parent_key": "task-auth-system",
      "estimated_minutes": 20,
      "description": "...",
      "acceptance_criteria": "...",
      "metadata": {
        "files": {
          "create": ["src/auth/oauth.ts"],
          "test": ["tests/auth/oauth.test.ts"]
        }
      }
    }
  ],
  "edges": [
    {
      "from_key": "task-2",
      "to_key": "task-1",
      "type": "blocks"
    },
    {
      "from_key": "task-1",
      "to_key": "task-auth-system",
      "type": "parent-child"
    },
    {
      "from_key": "task-2",
      "to_key": "task-auth-system",
      "type": "parent-child"
    }
  ]
}
```
