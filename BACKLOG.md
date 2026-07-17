# Backlog

Two sections, deliberately separate:

- **`## Tasks`** — planned, drainable dev work: things decided and ready to pull
  and implement. This is the ready-queue.
- **`## Deferred`** — work we consciously decided NOT to do now, kept so it is not
  forgotten. Each entry records what it is, why it was deferred, and any dependency
  that must land first.

Parked *ideas* (maybe-someday, not committed) live in [`IDEAS.md`](IDEAS.md), not
here. The full capture → classify → plan → drain flow is documented in
`llm-workflow/planning-pipeline.md`. **Backend for this project: markdown** (this
file + `IDEAS.md`) — a test round of the pipeline; may switch to `br` beads later
via a mechanical `beads-workflow` conversion. (The product's `bd`/orchestrator
store is a separate, untouched concern.)

## Tasks

Planned dev work, ready to pull. (Mirrors the kernl-tagged items in the
orchestrator inbox; this file is the markdown source of truth during the test.)

### Add a delete-task button + API
`PATCH /api/tasks` accepts only `status`/`tags`/`dueDate` and there is no `DELETE`
at all — so a task can be neither deleted nor retitled, by API or UI. For a task
manager this is a bug, not backlog. (Moved from `llm-workflow/BACKLOG.md` P1.)

### Add a field that lets a task be automatically developed by the orchestrator
A per-task flag marking it as auto-developable, so the orchestrator can pick it up
and drive it — the first concrete step toward developing kernl inside kernl.

### Review/redo kernl's UI — sidebar and palette
The sidebar (icons + logo) and the color palette.

### Batch override of auto-classify in the inbox
A checkbox to toggle whether auto-classify runs, plus a button to trigger the
classifier on the currently-unclassified inbox items.

### Add a way to organize and categorize projects
Some structure for grouping/categorizing projects.

### Add an inbox filter by classification
Filter the inbox by classification.

### A backlog/deferred section in the UI, separate from tasks
Surface a backlog/deferred area distinct from active tasks (the product-side mirror
of this file's Tasks-vs-Deferred split).

### Populate kernl's Memory with the telos
There is no `telos`-tagged note in `~/vault`, so the DA knows *what* exists and
stays blind to *why* (U7 of the v0.1.0 plan was never populated). (Moved from
`llm-workflow/BACKLOG.md`.)

### Give the DA the context this repo's agents already have
Carry over context *for the DA inside kernl* — logs, lessons, backlog, about-me —
as two problems that must NOT share one solution:
- **Constitutional (always-on):** who Gabriel is and what he aims at. This is what
  the `telos` tag + `MaxTelosBytes = 4000` cap was built for. The work is
  **curation** (distil `about-me/` to earn its 4 KB), not raising the cap.
- **Situational (on demand):** lessons, backlog, ops logs, session history —
  **retrieval, not injection**. Kernl already has `Classifier.relatedContext`
  (`internal/inbox/classifier.go`); the DA needs the equivalent tool.

Decided (2026-07-14): **llm-workflow stays the source of truth**. A symlink into
the vault is dead (`WalkDir`/`fsnotify` don't follow links; `LoadTelos` filters on
a `tags: [telos]` frontmatter that `about-me/*.md` lack). So build a **one-way
syncer (llm-workflow → kernl vault)** that stamps the frontmatter and curates to
the 4 KB cap — same shape as `sync-machine-rule.sh`. Do not let it grow into a
"sync everything" script. Code refs: `internal/chat/engine.go`,
`internal/planning/telos.go`. (Moved from `llm-workflow/BACKLOG.md` S10.)

## Deferred — from v0.1.0 (decided 2026-06-26)

> Source of the v0.1.0 scope decisions these defer from: the v0.1.0 roadmap
> brainstorm (2026-06-26). In-scope work is tracked separately as the v0.1.0 plan.

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

### ~~Bookmarks — full reformulation~~ → **UN-DEFERRED 2026-07-14. This is active work now.**
~~The bookmarks visualization is poor and needs a redesign.~~
- ~~**Why deferred:** off the magic loop; low priority. Ships rough in v0.1.0.~~
- **Why it came back (Gabriel, 2026-07-14):** he started processing his real WhatsApp
  inbox, and **the captures that are links have nowhere correct to land.** Saving them
  into today's bookmarks would mean re-doing all of it after the reformulation — so the
  reformulation is now *cheaper than the workaround*. **The deferral said "off the magic
  loop". Processing the inbox put it ON the magic loop.** That is the condition that
  expired, and it expired for the right reason.
- **Also, "ships rough" understated it:** the backend does not do what he wants yet —
  *"there are maaaany features to implement"*. What exists: `internal/graph/nodes/bookmark.go`
  (+ `bookmark_list.go`, tested), `internal/api/bookmarks.go` (create, list, highlights,
  archive), `cmd/kernl/bookmark.go` (`add`, `import`), and a UI of ~300 lines
  (`web/pages/bookmarks.vue`, `BookmarkItem.vue`, `BookmarkReader.vue`). So this is a
  **redesign on top of a working skeleton**, not a greenfield — and not a polish job either.
- **Blocked on a decision, not on code:** the development process itself. See
  `llm-workflow/BACKLOG.md` **S12** — there is no "loop" skill between bootstrap and review,
  and the sequencing questions this feature raises (backend or UI first, harden before or
  after) are exactly the gap. **This feature is the observation vehicle for S12: do the work
  watched, and write down what was actually done.**

### Inbox — the batch merge misses messages it should catch, AND it cannot tell you when it failed

**Gabriel, 2026-07-14:** the WhatsApp batch does not propose merging messages sent in sequence about
the same subject. **Investigated against the running server (`localhost:8080`), not by reading code —
and the result contradicted the first two theories, so the evidence is written down here in full.**

**What was tested.** Four real messages of his, posted to `/api/inbox/batch/analyze`:

```
6/13/26, 14:16 - Gabriel: coisa pra eu lembrar: … criar forma de gerenciar vagas:
                          - step (entrevista com IA, teste de código, …)
                          - detalhes da vaga (link, salário, …)
6/13/26, 14:16 - Gabriel: - itens que bianca colocou no curriculo heh   ← a forgotten bullet
4/6/26,  09:35 - Gabriel: https://claude.ai/chat/3d0b7342-…
4/6/26,  09:35 - Gabriel: PROCESSAR ^                                    ← acts on the previous msg
```

**Result:** `"enrichmentStatus": "applied"` — **the enricher runs, it is not broken** — and it
proposed exactly one merge:

```json
{"sourceSequences":[0,1], "reason":"second message adds a bullet to the list started in the first"}
```

**So three things are now known, and two of them killed a theory:**

1. **The forgotten-bullet case already works today.** It is an explicitly permitted merge in the
   current prompt (`internal/inbox/batch_enrichment.go:148` — *"an item added to a list an earlier
   message started"*). **No prompt change is needed for it.** The first theory (*"the prompt is too
   strict, loosen it"*) is **wrong for this case**.
2. **The `link` + `PROCESSAR ^` case is genuinely NOT covered.** A message that *refers to or acts on*
   its neighbour ("^", "isso", a link followed by an instruction) is **one thought split across two
   sends**, and it matches none of the three permitted shapes (restated / finished in the next message
   / bullet added to a list). This is a real prompt-coverage gap.
3. **A second, unrelated asymmetry:** the merge prompt renders messages as `[seq] Sender: body` —
   **with no timestamp** — while the classifier that runs later over the same captures *does* get one
   (`internal/inbox/classifier.go:327`: `[%d] (written %s)`). The merge proposer is strictly blinder
   than the classifier. Both of Gabriel's pairs were sent in the **same minute**, and the merge
   proposer cannot see that.

**THE TEST THAT STILL HAS TO BE RUN — this is the whole point of this entry.** If the bullet case
merges in a 4-message paste but did **not** merge in Gabriel's real paste, then **the prompt is not
the bug** and patching it would be treating a symptom. Two live hypotheses:

- **Scale.** In a paste of 100+ messages the model's attention degrades: a merge it finds trivially in
  4 messages it loses in 100. **If this is it, no prompt edit helps** — the fix is windowing/chunking
  the enrichment call.
- **The UI swallowed it.** See the Fail-Loud bug below.

**How to run it (Gabriel deferred this on 2026-07-14 — it is slow to reproduce by hand, so do it from
the API):** recover the original paste — **`batch_log` persists `RawText`** (`internal/inbox/batch_log.go`),
so any past `batchId` gives the exact text back — and POST it to `/api/inbox/batch/analyze`. Read
`enrichmentStatus` and `mergeProposals`. **Do not touch the prompt before this run answers
scale-vs-UI.**

**Then, and only then:** the fix for (2) and (3) is narrow — send the timestamp, and add the
"refers to / acts on the neighbour" shape to the permitted list. **Keep** *"same topic is NOT enough"*:
that half of the fence is still right, and the structural guarantees behind it (bodies rebuilt from
source; the human accepts every merge; the capture count is the parser's, not the model's — all landed
together in `dc2d888`) are what make a *proposal* safe in the first place.

### Inbox — the batch review UI hides its own failures (violates Fail Loud, Never Silent)

**Found 2026-07-14 while investigating the merge complaint above. This is a bug on its own, and it is
the reason the merge complaint was hard to diagnose at all.**

The backend computes an honest `EnrichmentStatus` — `applied | unavailable | failed | none` — plus an
`enrichmentError`. **The UI throws both away:**

- **`web/components/inbox/InboxBatchDump.vue:291-294` catches every `/analyze` failure and sets
  `proposals.value = []`.** An empty `catch`. A timeout, a 500, a model error, unparseable JSON — all
  render identically to *"the model looked and found nothing worth merging"*.
- **`enrichmentStatus` is never rendered anywhere in `web/`** (grepped: zero hits). If the LLM is not
  configured, `buildOptionalBatchLLM` returns `nil` → status `unavailable` → **zero merge proposals,
  silently.**

**So there is no way for the user to distinguish "no merges suggested" from "enrichment never ran".**
That is precisely what `AGENTS.md:90` forbids: *"**Fail Loud, Never Silent** … NEVER return a fallback"*.

**Fix:** surface the status in the review modal. `failed`/`unavailable` must *say so* ("merge
suggestions unavailable — the model call failed"), distinctly from `applied` with an empty list ("no
merges suggested"). The batch itself stays usable either way — enrichment genuinely is optional — but
the user has to be told which world they are in.

**Not started (Gabriel deferred it 2026-07-14: document, don't fix now).**

### Inbox — import a WhatsApp export *with* its media (zip), not just the text

**Gabriel, 2026-07-14.** Today the batch importer takes pasted text only. A real WhatsApp
export is a **directory**: the messages plus every attachment. Two things follow, and the
first is nearly free while the second is a genuine feature.

- **The cheap half — stop creating garbage captures.** There is currently **no handling of
  media lines at all** (grepped `internal/` and `web/` for `omitted|attached|Media`: zero
  hits). Every media message becomes a capture whose body is a filename or "Media omitted".
  Dropping those is a filter in `cleanSegments` (`internal/inbox/batch.go:695`).
- **The real half — the attachments are *pointers*, and they resolve.** Verified against
  Gabriel's own export at **`~/Downloads/whatsapp-export/`** (98 files + `messages-export.txt`).
  The Android format names the file **inside the message line**:

  ```
  4/5/26, 19:26 - Gabriel Assis: IMG-20260405-WA0009.jpg (file attached)
  ```

  The filename is the join key — **matching by timestamp is the fragile solution to a problem
  that does not exist.** (A caveat that will bite: WhatsApp prefixes these lines with an
  invisible **U+200E LRM** in some exports/locales, so a naive `HasPrefix` fails silently.
  Normalize first. The iOS/pt-BR variants use `<attached: …>` / `<anexado: …>` — do not
  hardcode one shape without checking against this dump.)

**The feature, as Gabriel scoped it:** upload the zip → match text to attachments → call an
**image-interpretation** model on the images → understand when several images were sent
**together** (the same dump has five images at `19:26`, which is exactly that case: they are
one thought, not five).

**Not started. The example dump to build against is `~/Downloads/whatsapp-export/` — read the
real format, do not write a regex against a remembered one.**

### Orchestrator — the decision artifact (autonomy without comprehension debt)

**Gabriel, 2026-07-14:** *"How do I use autonomous orchestration without comprehension debt?"*

**The rules already answer it — what is missing is a place to put the answer.** `AGENTS.md:110`
already says *"**Comprehension Debt:** never make a silent architectural decision… record the
rationale"*, and `AGENTS.md:113` already says a multi-domain edit is flagged **BLAST RADIUS** and
**never merged autonomously**. So the posture is settled: **execution may be autonomous; decision
may not.**

**The gap is that "record the rationale" has no destination.** It is an instruction with no
artifact, and an instruction with no artifact does not survive a fan-out of parallel agents —
the rationale ends up in a session that dies, and the reviewer is left reconstructing *why* from
a diff. That is comprehension debt arriving in the worst possible form: after the fact.

**The shape of the fix (not designed yet, deliberately):** every autonomously-executed unit
(bead/epic) must deposit, next to its output, what it **decided**, **why**, and **what it
rejected**. Comprehension debt is not paid by supervising more — it is paid by making the *why*
a required output, not a courtesy.

**Not started.** Related: the wider "what is the development loop" question is
`llm-workflow/BACKLOG.md` **S12**.

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
