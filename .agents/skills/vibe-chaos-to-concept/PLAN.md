# Vibe-Chaos-to-Concept Self-Critique Fix Plan

## Goal
Fix all remaining gaps identified in the last self-critique and run a second self-critique review to verify alignment with original plugin sources.

## Context
The vibe-chaos-to-concept plugin is a 3-stage pipeline (vc-ideate → vc-brainstorm → vc-plan) derived from upstream skills:
- ce-ideate → vc-ideate
- superpowers/brainstorming → vc-brainstorm
- planning-with-files → vc-plan

The last self-critique identified files to fix, brand leakage to fix, and concept drift to address. Several items remain. This plan fixes them all.

---

## Phase 1: Mechanical / Script-Level Fixes

### Task 1.1: Fix `spec-document-reviewer-prompt.md` — brand leakage
**File:** `skills/vc-brainstorm/references/spec-document-reviewer-prompt.md`
**Problem:** Contains legacy path `docs/superpowers/specs/`.
**Expected result:** Brand-neutral path `docs/`.
**Acceptance criteria:** The `Dispatch after:` line reads `docs/` not `docs/superpowers/specs/`.

### Task 1.2: Fix `frame-template.html` — remaining brand leakage
**File:** `skills/vc-brainstorm/scripts/frame-template.html`
**Problem:** Title and header still say "Superpowers Brainstorming" with a link to `obra/superpowers`.
**Expected result:** Title and header changed to `vc-brainstorm Visual Companion`, external repo link removed.
**Acceptance criteria:** No occurrence of "Superpowers" anywhere in the file; no link to `obra/superpowers`.

### Task 1.3: Fix `planning-with-files` in vc-plan script comments
**Files:**
- `skills/vc-plan/scripts/resolve-plan-dir.sh`
- `skills/vc-plan/scripts/resolve-plan-dir.ps1`
- `skills/vc-plan/scripts/attest-plan.sh`
- `skills/vc-plan/scripts/set-active-plan.sh`
- `skills/vc-plan/scripts/set-active-plan.ps1`
- `skills/vc-plan/scripts/session-catchup.py`
**Problem:** Each script file comment header contains the old upstream name `planning-with-files`.
**Expected result:** All file headers should use `vc-plan` instead (or be removed if they serve no purpose).
**Acceptance criteria:** No file in `skills/vc-plan/scripts/` contains the string `planning-with-files`.

### Task 1.4: Fix remaining `compound-engineering` brand leakage
**Files:**
- `skills/vc-ideate/SKILL.md` line 33 — example path `plugins/compound-engineering/skills/`
- `skills/vc-ideate/agents/ce-learnings-researcher.agent.md` line 76 — example search pattern `compound-engineering`
**Problem:** Example paths and search patterns leak the upstream repo name.
**Expected result:** Replace with neutral placeholder (e.g., `my-project`, `my-module`).
**Acceptance criteria:** No occurrence of `compound-engineering` anywhere in the plugin tree.

---

## Phase 2: Concept / Documentation Drift

### Task 2.1: Fix Yegge Loop description in AGENTS.md
**File:** `vibe-chaos-to-concept/AGENTS.md`
**Problem:** Two locations still describe an inaccurate iteration model:
- Line 87: "Runs the **Yegge Loop**: up to 5 iterative refining cycles..." 
- Line 103: "Maximum 5 iterations. Stop on contradiction."
**Expected result:**
- Line 87: "Runs the **Yegge Loop**: minimum 5 iterative refining cycles with user feedback, no hard cap..."
- Line 103: "Minimum 5 iterations. Continue if the user has feedback, no hard cap. Stop on contradiction."
**Acceptance criteria:** `grep` for "Maximum 5" returns zero matches in `AGENTS.md`.

### Task 2.2: Fix Yegge Loop description in top-level SKILL.md
**File:** `vibe-chaos-to-concept/SKILL.md`
**Problem:** Line 74 says "up to 5 iterations".
**Expected result:** "minimum 5 iterations, continue if the user has feedback, no hard cap"
**Acceptance criteria:** `grep` for "up to 5" in `SKILL.md` returns zero matches; the new phrasing is present.

### Task 2.3: Fix Yegge Loop description in vc-plan/SKILL.md
**File:** `skills/vc-plan/SKILL.md`
**Problem:** Three locations still use the old model:
- Line 290: "you MUST run up to 5 iterative refinement cycles"
- Line 315: "After 5 iterations, the plan has received substantial review." (this is actually correct per v2.37 but the intro line needs to match)
- The "up to 5" phrasing misrepresents the actual Yegge Loop rule (≥5, no hard cap).
**Expected result:**
- Change "you MUST run up to 5 iterative refinement cycles" → "you MUST run at least 5 iterative refinement cycles"
- Keep line 315 as-is (it's fine after a minimum of 5).
**Acceptance criteria:** "up to 5" no longer appears in the Loop Protocol section.

---

## Phase 3: Structural / Alignment Gaps

### Task 3.1: Reference `spec-document-reviewer-prompt.md` from `vc-brainstorm/SKILL.md`
**File:** `skills/vc-brainstorm/SKILL.md`
**Problem:** The spec reviewer template exists but is unreferenced in the skill file, just like the original. Unlike the original, we can add a lightweight reference in the **Spec Self-Review** section to indicate it is optionally available for sub-agent dispatch.
**Expected result:** In the Spec Self-Review section, add a one-sentence pointer: for deeper review, dispatch a reviewer using the template at `references/spec-document-reviewer-prompt.md`.
**Acceptance criteria:** The template is mentioned once in `vc-brainstorm/SKILL.md`.

### Task 3.2: Verify 9-step checklist numbering in `vc-brainstorm/SKILL.md`
**File:** `skills/vc-brainstorm/SKILL.md`
**Problem:** The self-critique noted the checklist uses a heading list instead of numbered steps. This is functionally equivalent (both are numbered lists), but we want to ensure the checklist is crystal clear.
**Expected result:** The checklist is a numbered markdown list (1–9) with bold item names.
**Acceptance criteria:** Lines 36–44 are a markdown numbered list `1. **...**` through `9. **...**`.

### Task 3.3: Remove remaining `superpowers`/`ce-ideate` from `AGENTS.md`
**File:** `vibe-chaos-to-concept/AGENTS.md`
**Problem:** Two file structure comments still leak source names:
- Line 113: `# Modified from ce-ideate`
- Line 119: `# Modified from superpowers/brainstorming`
**Expected result:** Remove or neutralize these comments (they are internal lineage notes that don't need to appear publicly).
**Acceptance criteria:** No occurrence of `ce-ideate`, `superpowers`, `compound-engineering`, or `planning-with-files` in `AGENTS.md`.

---

## Phase 4: Final Self-Critique

### Task 4.1: Run full-text scan for brand leakage
**Command:** `rg -i 'superpowers|compound.engineering|planning-with-files|ce-ideate|ce-plan|ce-brainstorm' vibe-chaos-to-concept/`
**Expected result:** Zero matches across the entire plugin tree.
**Acceptance criteria:** The grep returns no results.

### Task 4.2: Compare concept drift against originals
**Files to compare:**
- `vc-brainstorm/SKILL.md` — verify Context Intake, artifact paths, checklist, Visual Companion reference
- `vc-plan/SKILL.md` — verify Context Intake, Yegge Loop rules, docs/ migration
- `vc-ideate/SKILL.md` — verify scratch path, Proof removal, agent names
**Expected result:** Alignment scores match or exceed previous report (vc-ideate ≥99%, vc-brainstorm ≥95%, vc-plan ≥90%).
**Acceptance criteria:** A written alignment assessment is produced with scores.

### Task 4.3: Document any remaining gaps
**Expected result:** A clear list of any gaps that remain after fixes, with rationale for why they were left untouched.
**Acceptance criteria:** The user sees a concise summary of what was fixed and what (if anything) was intentionally not changed.
