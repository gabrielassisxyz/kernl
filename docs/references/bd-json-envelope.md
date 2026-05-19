# BD JSON Envelope Migration Reference

> Source: beads CLI migration doc. Captured here so the Kernl repo stays self-contained.

## Context

`bd` 1.0.4 emits a deprecation notice when `--json` is used without the env var `BD_JSON_ENVELOPE=1`. In v2.0 the envelope format becomes the default. This document summarises the schema change so Kernl can migrate its parsers without depending on an external clone of the beads repo.

## Envelope format (`BD_JSON_ENVELOPE=1`)

Every `--json` response is wrapped uniformly:

```json
{
  "schema_version": 1,
  "data": <original-payload>
}
```

- The payload inside `.data` is untouched — no field injection, no type coercion.
- Arrays are wrapped the same way: `{ "schema_version": 1, "data": [ ... ] }`.
- `schema_version` is an integer that increments only on breaking changes (renames, removals, nesting depth shifts). Additive changes do **not** bump the version.

## Legacy format (default until v2.0)

- **Object commands** (`show`, `create`, `update`, `close`, …): emit a plain JSON object *without* the `.data` wrapper. `schema_version` is a top-level field alongside the data fields.
- **List commands** (`list`, `ready`, `blocked`, `search`, …): emit a raw JSON array with no wrapper at all.

## Migration checklist for consumers

1. Set `BD_JSON_ENVELOPE=1` in the environment before invoking `bd`.
2. After reading stdout, check whether the root is an object with a `data` key.
   - If yes: unwrap `.data` before unmarshalling into your structs.
   - If no: fall back to legacy parsing (direct unmarshal).
3. Compare `schema_version` against the version you expect. If it is higher than expected, log a warning but continue parsing (additive changes are backwards-compatible).
4. Errors on stderr with `--json` active are **not** wrapped in the envelope; they remain plain JSON objects like `{ "schema_version": 1, "error": "...", "code": "..." }`.

## Quick diff

| Command | Legacy path | Envelope path |
|---------|-------------|---------------|
| `bd show <id> --json` | `.title` | `.data.title` |
| `bd list --json` | `.[0].id` | `.data[0].id` |
| `bd ready --json` | `.[0].title` | `.data[0].title` |
| `schema_version` | top-level | top-level |
