# Backlog

Deferred work — things we consciously decided NOT to do now, kept here so they
are not forgotten. Each entry records what it is, why it was deferred, and any
dependency that must land first.

> Source of the v0.1.0 scope decisions these defer from: the v0.1.0 roadmap
> brainstorm (2026-06-26). In-scope work is tracked separately as the v0.1.0 plan.

## Deferred from v0.1.0 (decided 2026-06-26)

### Ingest — "Deep Research" action
When ingested content makes a claim needing verification, this action would
dispatch a research task (an agent investigates external sources and folds
findings back into the graph).
- **Why deferred:** needs a research-agent pipeline that does not exist; overlaps
  with the Orchestrator / DA tools, both deferred.
- **Depends on:** agent research pipeline (Orchestrator or a DA research tool).
- **Note:** the extractor prompt is being narrowed to stop emitting this action
  until it is built; re-enable in `internal/ingest/llm_extractor.go` when ready.

### Ingest — "Add Contradiction Callout" action
When ingested content conflicts with itself or with established knowledge, this
action would flag the conflict (attach a contradiction callout) for reconciliation.
- **Why deferred:** needs contradiction detection/marking infrastructure, which is
  conceptually the same machinery as Memory's `refutes` edges, plus a callout UI.
- **Depends on:** Memory rewiring (v0.1.0). Candidate **stretch** once Memory is
  wired and time allows; otherwise a follow-up.

### Notes — Undo delete note
A feature to undo the deletion of a note (move the note back from the system trash to the vault and reconnect it in the graph).
- **Why deferred:** the initial implementation simply moves the file to the system trash. Building a full undo flow (tracking the original location, moving it back, and ensuring graph reconnection) is a follow-up scope.

### Notes — WYSIWYG / ProseMirror editor (Tiptap / Milkdown)
A true rich-text editor beyond the v0.1.0 CodeMirror 6 live-preview approach.
- **Why deferred:** CodeMirror live-preview is the closest-to-Obsidian path and
  reuses the existing editor; a ProseMirror swap is a larger rewrite with markdown
  round-trip risk. Only pursue if live-preview proves insufficient in daily use.

### DA — automated eval harness (golden transcripts, scored)
Replace/augment the manual UAT plan with scored, repeatable evals.
- **Why deferred:** premature carrying cost before prompts stabilize. Build after
  the manual UAT (v0.1.0) settles the prompts.

### Bookmarks — full reformulation
The bookmarks visualization is poor and needs a redesign.
- **Why deferred:** off the magic loop; low priority. Ships rough in v0.1.0.

### Orchestrator — polish / real-world mileage
Hardening the epic-to-PR path against live agent CLIs.
- **Why deferred:** low priority for v0.1.0; the loop does not depend on it for
  daily solo use.

### Settings + Profile — dedicated UI
No UI exists for settings or profile today.
- **Why deferred:** not on the magic loop; not a v0.1.0 priority.

### Graph — full rework (clustering, hover edge labels)
Richer graph interactions beyond the v0.1.0 visibility canary.
- **Why deferred:** the canary (edge visibility + verifying connections actually
  work) covers v0.1.0; clustering/labels are over-scope.

### Memory — claim editing / versioning UI
Anything beyond Keep / Edit / Discard on the `DA · learned` card plus the existing
refute flow.
- **Why deferred:** YAGNI until a real need shows up.

## UI chrome currently mocked (decided 2026-06-28)

These ship as hardcoded placeholders because no real data source exists yet. Kept
intentionally (they read as "real" chrome) until the backing state is built; replace
the mock when wiring each.

### DA panel — version badge, scope, and context chips
`web/components/DaChatSurface.vue`: the `v2.4-stable` version badge (rendered per
assistant message — so it only shows once the DA replies, which is why an empty chat
shows nothing), the `scope · global` chip, and the `Context: All files` footer chip are
all static.
- **Why deferred:** there is no versioning, scope, or active-context concept wired to
  the DA yet. Define those concepts, then bind the chips.
- **Note:** the greeting name (`Morning, Gabriel.`) is also hardcoded; source it from
  user/session config when that exists.

### Shell footer — `system_online` and `synced` status
`web/layouts/default.vue`: the green `system_online` dot/label and the `synced` indicator
are static.
- **What to build:** a real status signal for each.
  - `system_online`: define the possible states (e.g. `online` / `degraded` / `offline`),
    derived from a health/heartbeat check (the `/api/health` endpoint already exists and
    now returns vault info — extend it with a real status, or add a dedicated probe).
  - `synced`: define the possible states (e.g. `synced` / `syncing` / `dirty` / `error`),
    derived from vault-watch / write-back state.
- **Why deferred:** no health-state or sync-state source exists yet; wiring a label to a
  nonexistent source would be a "live lie".
- **Done already (not mocked):** the `~/vault` path now comes from real config via
  `/api/health` → `vaultLabel`; the `UTF-8` and `system_ready` chips were removed.
