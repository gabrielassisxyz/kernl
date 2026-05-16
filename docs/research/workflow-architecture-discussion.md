# Workflow Architecture Discussion — Kernl Orchestrator

> **Status:** Tentative proposal — input for the upcoming `vc-brainstorm` session on Kernl vision. **Not** a decision document.
> **Date:** 2026-05-15
> **Context:** Captured from a discussion working through multi-agent orchestration patterns, comparing Gastown and NTM, and converging on a working architecture for Kernl.

---

## Why this document exists

The user (solo developer building Kernl) reported lacking clarity on the "state of the art" / final-system vision for Kernl, which is blocking decisions about next steps. Before opening a dedicated `vc-brainstorm` session, this discussion stress-tested the user's architectural intuitions about multi-agent orchestration patterns by:

1. Defining the design space (3 canonical patterns, later expanded to 4).
2. Analyzing how Gastown and NTM solve the same problem with opposite philosophies.
3. Running a pros/cons / wins-and-losses adversarial pass to check for bias.
4. Converging on a working position to take into the brainstorm.

The user explicitly identified a personal bias toward role-based patterns and asked for adversarial pushback. The result is the proposal below.

---

## 1. The four architecture patterns (design space)

### 1.1 Fungible-generalist
All agents are identical, interchangeable workers pulling from a shared task pool. No specialization. Coordination is external (operator / dispatcher / graph triage). Failure-resilient and linearly scalable. Anti-pattern: "communication purgatory" (agents talk forever without producing).

**Analogy:** Uber drivers — anyone takes any ride. Checkout cashiers — open another lane when busy.

**Example in the wild:** NTM (`ntm spawn payments --cc=3 --cod=2 --gmi=1` — model diversity is **not** specialization).

### 1.2 Role-based by *domain*
Agents specialize by knowledge area (frontend agent, backend agent, ML agent, security agent). Each has different prompts, tools, possibly different models.

**Analogy:** Chef de partie in a restaurant — fish station, meat station, pastry station. Orchestra — first violin, percussion.

**User's stance:** Rejected. "Domain roles don't make sense for code work."

### 1.3 Role-based by *workflow stage*
Agents specialize by their position in the pipeline (planner, implementer, reviewer, merger, integration-reviewer, releaser). The **implementation tier** can be fungible internally — what's specialized is the workflow stage, not the work itself.

**Analogy:** Assembly line — workers are interchangeable within a stage, but each station has a specific function. Military platoon — every soldier is a rifleman (fungible base), but there are medics, comms, snipers (specialists).

**Example in the wild:** Gastown — Mayor (coordinator), Polecats (fungible implementers), Witness/Deacon (monitoring), Refinery (merge queue), Crew (human workspace).

### 1.4 Dispatch-routed hybrid (the user's synthesis)
Pool of fungible workers + library of workflow shapes. At dispatch time, the orchestrator classifies the task and routes:
- "Trivial / quick task" → 1 fungible worker, direct flow.
- "Complex feature" → full pipeline (planner → impl → review → merge → integration-review).
- "Specialized task" → on-demand specialist role (sweeper, auditor, deployer).

**Cost:** The routing logic is itself a planning step. Either tasks come pre-tagged or the orchestrator must be smart enough to classify.

**Benefit:** Don't pay pipeline overhead on trivial work; don't lose pipeline rigor on complex work.

---

## 2. Gastown vs NTM — concrete comparison

| Axis | Gastown | NTM |
|---|---|---|
| Pattern | Role-based by workflow stage | Fungible-generalist |
| Mental model | "City with departments" | "Pool + dispatcher" |
| Coordinator | Mayor (dedicated agent) | Operator (human + CLI surface) |
| Merge handling | Refinery (dedicated role, Bors-style queue) | External CI / operator |
| Health monitoring | Witness + Deacon + Dogs (3 roles) | Convergence triple-check + operator tick |
| Specialist failure | Witness/Deacon try to recover | Not applicable (no specialists) |
| Scaling shape | Polecats scale linearly; specialists stay 1-of-each | Linear across the whole pool |
| Conceptual complexity | High (8+ distinct concepts) | Medium (pool + dispatch + safety) |
| Agent identity | Persistent (Polecats have history) | Ephemeral / interchangeable |
| Coordination locus | Distributed across specialized roles | Centralized in graph (`bv`/`br`) + operator |

**Shared substrate:** Both use tmux, Go, Beads, `bv`. Both support multi-model providers. Both expose CLI + robot/JSON surfaces. The implementations are similar — the **philosophies** are opposite.

---

## 3. Adversarial pros/cons — what survived the back-and-forth

### Where fungible-generalist clearly wins

1. **Bursty/unpredictable workload shape** — *Caveat: weakened by well-scoped beads with full acceptance criteria. Properly-decomposed work neutralizes day-to-day variation.*
2. **High agent churn** — Each death absorbed without role-replacement.
3. **Don't-know-the-roles-yet projects** — *Caveat: broad-stroke roles (reviewer, merger, deployer) are predictable for any serious project. Only granular sub-roles are unknown upfront.*
4. **Cross-cutting refactor across domains** — *Caveat: context window limits make "one agent follows the whole thread" rare in practice.*
5. **Solo dev, small swarm** — Less conceptual load.
6. **Low task count** — Doesn't waste specialist slots.

### Where role-based by workflow stage clearly wins

1. **Tasks with fundamentally different semantics from "normal work"** — Merge queue needs ACID + linearization (Bors-style). Trying to make a generalist do it produces races. **Strong argument.**
2. **Model arbitrage** — Reviewer can use cheap model; implementer needs powerful model. Naturally routed by stage.
3. **Compliance / separation of duties** — "Who writes doesn't review" enforced structurally.
4. **Debug clarity** — Knowing which **capability** is impaired ("merges are stuck") beats knowing which agent instance failed.
5. **Stateful long-running supervisors** — Accumulate baseline knowledge of "what's normal".
6. **Ambient/maintenance work** — Needs explicit owner; "anyone" = "nobody".

### Failure modes (the ones that bite *later*)

**Fungible failure modes:**
- Communication purgatory.
- Orphaned cross-cutting tasks (sweep, stale detection, convergence — no one owns them).
- Per-agent context bloat (every generalist must load whole project).
- Coordination becomes the actual bottleneck (mail + reservations → contention).

**Role-based failure modes (mitigations identified in discussion):**
- Single-point-of-failure per role — *Mitigated by heartbeats and supervisor-of-supervisor (Deacon → Witness, dashboard).*
- Role drift over time — *Mitigated by clean boundary design; not inevitable.*
- New role expensive to add — *Reality check: ~half is boilerplate, and adding implies real bottleneck identified.*
- Specialist underutilization — *Mitigated by on-demand instantiation (not always-on).*

### Concrete scenario winners

| Scenario | Winner |
|---|---|
| Solo dev, greenfield, 4-6 agents, varied work | Fungible (simpler start) |
| Production codebase, SLA, audits, 5+ devs | Role-based (Refinery + Reviewer separation) |
| Multi-repo, predictable categorized work | Role-based (marginal) |
| Cross-cutting 200-file refactor | Fungible (if context window permits) |

---

## 4. The user's working position

After the adversarial pass, the user converged on:

> **Workflow-staged, fungible-within-stage, dispatch-routed, on-demand specialists.**

Concretely:
- **Pool of fungible workers** at the execution tier (no "frontend agent").
- **Workflow-stage roles** around the pool — planner, reviewer, merger, integration-reviewer, sweeper — **instantiated on-demand, not always-on**.
- **Dispatch-time routing** — orchestrator classifies the task and decides whether to apply full pipeline or a short flow.
- **Mayor-style conversational coordinator** (action channel) **+** dashboard (passive observability) — both, not either-or.
- **No domain-roles.**
- **No always-on watchdogs** (Witness/Deacon-style) — solo-dev scale makes them overkill; heartbeat + dashboard covers monitoring.

---

## 5. Proposed workflow stages

### Hot path (per-épico)

| # | Stage | Purpose | Notes |
|---|---|---|---|
| 1 | **Planner** | Decompose épico into beads with acceptance criteria; build DAG | May skip for trivial work |
| 2 | **Implementer** (fungible pool) | Pick available bead, execute | Where parallelism actually scales |
| 3 | **Per-bead Reviewer** | Review individual bead output; approve/reject | Loop-back protocol below |
| 4 | **Merger** | Merge approved bead into épico branch (Bors-style) | Single-flight (one merge at a time per épico) |
| 5 | **Integration Reviewer** | Review the merged whole after all children landed | Catches "parts work, integration breaks" — gap Gastown's Refinery doesn't fill |
| 6 | **Releaser** (optional) | Versioning, changelog, final PR to main | Skip if internal-only work |

### Continuous / on-demand (outside hot path)

| # | Stage | Purpose |
|---|---|---|
| 7 | **Sweeper** | Ambient work: stale issue detection, worktree cleanup, bd↔git sync, zombie agent reaping. Already has `internal/sweep/` package as basis. |
| 8 | **Auditor** (multi-mode) | Full-codebase analysis under different lenses: code quality, security, performance, test coverage/quality, docs completeness |

### Stages considered but parked

- **Brainstormer / Strategizer** — Pre-planner. For turning vague ideas into planning-ready épicos. The user does this manually with `vc-brainstorm` today; may formalize later.
- **Spec-writer** — Between Planner and Implementer. Probably unnecessary if Planner produces good acceptance criteria.
- **Test-writer / Doc-writer** — Rejected as separate roles; these are just beads with acceptance criteria, executed by the fungible pool.

---

## 6. Rejection / loop-back protocol (critical detail)

> **Status:** Both protocols below (per-bead loop with escalation threshold; Integration Reviewer fix-up beads) are **user-confirmed decisions**, not open proposals. The escalation threshold value (currently N=3) is still open — see §8 question 5.

### Per-bead Reviewer rejects

**Default flow:** bead returns to the pool with reviewer's feedback attached.
- Bead state: `needs_review` → `ready` (with `review_feedback` field populated).
- Any implementer picks it up (could be same or different — fungibility means it doesn't matter).
- Reviewer feedback enters the implementer's initial context.
- Cycle: implementer → reviewer → (reject) → implementer → reviewer → (approve).

**Guard-rails (mandatory to prevent infinite loops):**
- `rejection_count >= N` (e.g. N=3) → escalate to human automatically.
- Recurrent pattern detected (same class of error twice consecutively) → escalate before N.
- Time-budget exceeded → escalate.

### Integration Reviewer rejects

**Critical difference:** does **not** reject the whole épico (would discard N children's work). Instead:
- Creates **fix-up beads** attached to the épico, describing what needs adjustment.
- Fix-up beads enter the normal pipeline (Planner or direct Implementer depending on complexity).
- When fix-ups close, Integration Reviewer re-runs.
- Same escalation guard-rails apply.

### Why this matters

Loop-back protocols are where badly designed orchestrators silently waste resources or get stuck. The escalation guard-rails should be specified **before** the loop is implemented, not added reactively after a stuck production loop.

---

## 7. CI vs Orchestrator stage split

Both layers run quality checks but serve different purposes:

| Layer | Primary use | Examples |
|---|---|---|
| **CI** | Deterministic checks, fast feedback, gated on merge | Unit tests, linters, formatters, build, type-check, basic security scanners |
| **Orchestrator (Auditor stages)** | LLM-reasoning analyses, full-codebase scans, deep dives, second-opinion when CI passed | Architectural review, semantic security audit, performance profiling under load, doc-completeness across the project, test-quality review (not just coverage) |

**Not mutually exclusive.** CI gates every PR; Auditor stages run on-demand (operator request) or scheduled (Sweeper detects "time to audit"). CI says "compiles and tests pass"; Auditor says "the code is good." Different questions.

---

## 8. Open questions to validate in brainstorm

These survived the discussion but were **not resolved** — they belong in the brainstorm input, not its conclusion.

1. **Dispatch-routing classification logic** — How does the orchestrator decide pipeline vs short-flow? Tasks come pre-tagged, or orchestrator infers from bead content? Both?
2. **Coordinator agent vs CLI surface** — Should Kernl have a "Mayor" agent (Gastown-style) or expose orchestrator state via CLI + dashboard only (NTM-style)? The user wants "both" but the implementation details matter.
3. **On-demand specialist lifecycle** — When a fix-up bead is needed, does the Integration Reviewer create the bead and exit, or stay alive supervising the loop? Stateless or stateful?
4. **Ambient work scheduling** — Sweeper runs how? Cron? Triggered by events? Periodically polled by the orchestrator?
5. **Per-bead Reviewer thresholds** — Is `N=3` rejections the right escalation threshold? Configurable per project or hard-coded?
6. **Multi-épico coordination** — All discussion has been within a single épico. What about cross-épico work? Shared dependencies?
7. **What about `Brainstormer` as a formal stage?** — The user uses `vc-brainstorm` manually for vision-level work today. Worth formalizing as a stage, or keep as human-driven precursor to Planner?
8. **The role of `bv` graph triage** — In the proposed architecture, where does `bv --robot-next` fit? Inside dispatch? Inside Planner?

---

## 9. What was NOT decided in this discussion

Listed explicitly so the brainstorm session doesn't accidentally treat these as settled:

- **Whether Kernl should ship with a Mayor-style coordinator agent at all.** The user wants conversational interface + dashboard, but whether the conversational interface is a *persistent agent* or *just a chat layer* over the orchestrator is open.
- **Whether on-demand specialists share infrastructure with the fungible pool** or run isolated.
- **The exact set of stages.** The 8 stages are a proposal, not a contract.
- **How this architecture composes with the existing `orchestrator/internal/` packages** (workflow, merge, sweep, dispatch, epic, etc).
- **State-of-the-art vision for Kernl as a whole** — the original gap. This document covers orchestrator architecture only; the broader vision (Kernl as Jira + Obsidian + chat + wiki + orchestrator, per `ideas.md`) was deliberately deferred.

---

## 10. Bias checks applied

The user explicitly identified a natural preference for role-based architectures and asked for adversarial pushback. The discussion:

- Forced the user to confront cases where fungible clearly wins (bursty work, cross-cutting refactor, solo-dev simplicity).
- Forced acknowledgment that several role-based arguments were weaker than they first appeared (specialist underutilization mitigated by on-demand instantiation; new-role cost lower than claimed; role drift not inevitable).
- Surfaced the **key reframing** that "role-based" was being used ambiguously to mean both "role-by-domain" (which the user rejects) and "role-by-workflow-stage" (which the user accepts). This was the most important clarification.
- Introduced the dispatch-routed hybrid as a fourth pattern the user effectively invented mid-discussion.
- Confirmed that "single-user" does **not** force fungibility — only removes pressure to scale.

The user's final position is a synthesized hybrid, not a clean acceptance of either Gastown or NTM. This is intellectually honest output, not bias confirmation.

---

## Cross-references

- `ideas.md` — Original vision/brainstorm prompt for Kernl as a whole.
- `docs/STRATEGY.md` — Current strategy doc (3 tracks, 4 metrics).
- `TODOS.md` — Deferred items (workflow refactor blocker, automation/onboarding, abort subcommand, etc).
- `BACKLOG.md` — Recent capture items (jeffrey-skills, MCP agent mail, claude-memory-bank, overprompting reference).
- `docs/research/jeffrey-skills-map.md` — Catalog of installed jeffrey-skills with relevance buckets for Kernl.
- `docs/plans/2026-05-15-kernl-workflow-plan.md` — Current implementation plan (the workflow-stage *primitives* implementation, separate from the architectural decision captured here).

The pattern decision in this document is **upstream** of the implementation plan. The plan implements primitives; this document discusses how those primitives compose.
