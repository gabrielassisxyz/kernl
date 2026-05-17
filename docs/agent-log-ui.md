# Agent Log UI

Live agent output in the web GUI — minimal chat-style view of what the model
is doing while a bead runs.

## What already exists (no changes needed)

| Layer | What's there |
|-------|-------------|
| `SessionRuntime` | reads agent stdout NDJSON line-by-line, emits `TerminalEvent` |
| `sessionPump` (driver.go) | reads `SessionRuntime.Events()` → `scm.HandleEvent()` |
| `SessionConnectionManager` | buffers + fan-out, replays on reconnect |
| `GET /api/sessions/{id}/events` | SSE endpoint, streams `TerminalEvent` JSON |
| `GET /api/epics/{id}/events` | already emits `SessionStarted` events carrying `sessionId` |
| Frontend `app.js` | already captures `sessionId` from epic SSE events into `sessions{}` map — just never uses it |

The data pipeline is complete. The frontend only needs to subscribe and render.

---

## What to build

### Backend (small)

One new endpoint so the frontend can recover session state on page refresh
(it already knows the session ID from the live SSE but loses it on reload):

```
GET /api/epics/{id}/sessions
→ [{ "sessionId": "kernl-xyz-opencode", "beadId": "kernl-xyz", "status": "running" }, ...]
```

Implementation: read `app.SCM.GetConnectedIDs()`, cross-reference with
`app.SCM.GetSessionEntry()` to attach `beadId`. ~30 lines in `api/epics.go`.

### Frontend

Split the existing single-column layout into two panels when a bead is selected:

```
┌─────────────────────┬────────────────────────────────────┐
│  Bead list (left)   │  Agent log (right)                 │
│                     │                                    │
│  ● kernl-xyz        │  Thinking                          │
│    active           │    Planning the grep pattern…      │
│                     │                                    │
│  ○ kernl-abc        │  I'll start by listing the files.  │
│    queued           │                                    │
│                     │  ▶ bash  ls orchestrator/          │
│                     │    ↳ cmd/ internal/ go.mod         │
│                     │                                    │
└─────────────────────┴────────────────────────────────────┘
```

Right panel only appears when an active bead is selected. Falls back to
"no active session" if the bead has no running session.

---

## Event data flow

Each SSE message from `/api/sessions/{id}/events` is a `TerminalEvent`:

```json
{ "type": "stdout", "content": "<raw NDJSON line>", "beadId": "kernl-xyz", "time": 1234567890 }
```

`content` is the raw NDJSON emitted by the agent. Parse it as JSON to get the
inner event. For `type != "stdout"` (e.g. `"stderr"`, `"exit"`), treat `content`
as plain text.

---

## Inner event types to handle

These are the agent-emitted types that appear in `content`:

### opencode dialect

| Inner `type` | What it means | How to render |
|---|---|---|
| `message.part.updated` with `part.type = "text"` | Model text output | White text, append as it streams |
| `message.part.updated` with `part.type = "reasoning"` | Thinking block | See below |
| `message.part.updated` with `part.type = "tool"` | Tool call | See below |
| `session_idle` | Agent finished turn | Show subtle separator |

### Claude Code dialect

| Inner `type` | What it means | How to render |
|---|---|---|
| `assistant` with content block `type = "text"` | Model text | White text |
| `assistant` with content block `type = "thinking"` | Thinking | See below |
| `tool_use` | Tool invoked | See below |
| `tool_result` | Tool output | See below |
| `result` | Turn complete | Show subtle separator |

Anything else: ignore silently. The frontend does NOT need to handle every
possible type — unknown types are dropped.

---

## Render spec

### Thinking block

```
Thinking                    ← "Thinking" label, color #f59e0b (amber)
  Planning the grep…        ← reasoning text, color #6b7280 (gray-500), italic
  The module path is…         indented 1em, slightly smaller font
```

Collapsed by default after the turn ends. Expanded while streaming.

### Regular text

```
Let me check the file structure first.   ← color #f3f4f6 (gray-100), normal weight
```

No label, no indentation. Appended as chunks arrive.

### Tool call

```
▶ bash                      ← tool name, color #60a5fa (blue-400), monospace
  ls orchestrator/          ← input (key args only), gray, truncated at 120 chars
  ↳ cmd/ internal/ go.mod   ← result, green (#4ade80), appears when tool completes
```

`▶` becomes `✓` (green) on success or `✗` (red) on error when result arrives.
Tool input and result are each truncated to 3 lines / 240 chars with a "…more"
toggle. The tool block is a single collapsed row once result is received.

### Stderr

```
[stderr] error message here   ← color #f87171 (red-400), monospace, small
```

### Turn separator

A faint horizontal rule between turns. No text.

---

## Implementation steps

1. **Backend** — add `GET /api/epics/{id}/sessions` endpoint (~30 lines).
2. **HTML** — add `<div id="log-panel">` to `index.html`, hidden by default.
3. **CSS** — split layout (CSS grid, two columns), log panel styles.
4. **JS: session lookup** — on bead click, call `/api/epics/{id}/sessions` to
   get `sessionId`; fall back to `sessions[beadId]` already in memory.
5. **JS: SSE subscription** — open `EventSource` on `/api/sessions/{sessionId}/events`,
   close previous one if another bead was selected.
6. **JS: event parser** — parse outer `TerminalEvent`, then parse `content` as JSON.
   Route to renderer based on inner type.
7. **JS: renderers** — one function per block type (thinking, text, tool, stderr).
   Append to log div. Auto-scroll to bottom unless user has scrolled up.
8. **JS: replay** — on connect, SCM replays buffered events automatically;
   frontend handles them identically to live events (idempotent).

Total: ~300 lines of new JS/CSS/HTML. No build toolchain changes.
