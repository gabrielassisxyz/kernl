# Spike: opencode run -s resume semantics

**Date**: 2026-05-15
**Spike**: Verify that `opencode run -s <session_id>` non-interactive resume works
as assumed by the SessionDriver design (Phase 2).

## Verdict

**Resume design confirmed.** `opencode run -s <sessionID>` correctly resumes a
prior session in non-interactive mode. The SessionDriver can spawn a new session
via `opencode run --format json <prompt>`, capture the `sessionID` from the first
`step_start` event, and issue follow-up turns via `opencode run -s <id> --format json <prompt>`.

## Flag spelling

| Flag | Description |
|------|-------------|
| `-s`, `--session` | Resume specific session by ID |
| `-c`, `--continue` | Resume the last session (convenience, no ID needed) |
| `--fork` | Fork session before continuing (requires `-s` or `-c`) |

## Key findings

### 1. Session ID capture
Every JSON event (`step_start`, `text`, `tool_use`, `step_finish`) carries
`"sessionID":"ses_..."` in its top-level fields. The first `step_start` event is
sufficient for capture — no separate parsing needed.

Session ID format: `ses_` + hex + nanoid-like suffix. Example:
`ses_1d63dad2effeZoaAco7Vtb73Qy`

### 2. Resume works non-interactively
```
opencode run -s "ses_1d63dad2effeZoaAco7Vtb73Qy" \
  --format json \
  --dir /tmp/repo \
  "what did I ask you earlier?"
```
The model correctly recalled prior context. Same `sessionID` in all response
events.

### 3. Invalid session behavior
```
opencode run -s "ses_nonexistent" --dir /tmp/repo "hello"
```
- Stderr: `Error: Session not found`
- Exit code: **1**
- Kernl's SessionDriver must handle exit code 1 by surfacing the error via
  `session-error` epic event and NOT retrying infinitely.

### 4. Session persistence
Session files stored at `~/.local/share/opencode/storage/session_diff/ses_*.json`.
Sessions persist across `opencode run` invocations (verified: session created in
one command, resumed in a separate command).

### 5. Event stream structure
Events are NDJSON (one JSON object per line). Key event types relevant to the
SessionDriver's take-loop:

| Event | Significance |
|-------|-------------|
| `step_start` | Turn began. Contains `sessionID` — capture point. |
| `text` | Model output text. Kernl routes to session buffer. |
| `tool_use` | Agent used a tool. May trigger approval flow. |
| `step_finish` | Turn ended. Contains `reason` (`stop`, `tool-calls`) and token counts. |

## Implications for SessionDriver (Phase 2, Tasks 22-27)

1. **Spawn**: `opencode run --format json --dir <worktree> <initial-prompt>`
2. **Capture**: Read first `step_start` event → store `sessionID` in run-state
3. **Follow-up turns**: `opencode run -s <id> --format json --dir <worktree> <nudge>`
4. **Error**: Exit code 1 → classify as `session-error`, surface to epic

## Limitations not tested

- Session expiry: do sessions expire after inactivity? Unknown — not within the
  spike window (~5 min between invocations works).
- Concurrent sessions: can two `opencode run` commands target the same session
  simultaneously? Not tested — Kernl's SessionDriver guards this at the
  orchestration layer via the lease system.
- `--fork` semantics: appeared unreliable in testing (sometimes "Session not
  found"). Kernl does not depend on `--fork` for MVP.
