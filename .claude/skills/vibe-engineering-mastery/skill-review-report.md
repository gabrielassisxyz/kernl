# Skill Review: vibe-engineering-mastery

## Health Score: 95/100

## Summary
A well-architected, 8-skill multi-stage planning pipeline. All structural validations pass. Trigger phrases were recently added to every skill, lifting all sub-skills to 90-100. Three instruction-dense review skills exceed the 5000-token guideline, but this is **accepted by design** after reviewing the authoritative context-window behavior from Anthropic's documentation (see "Accepted Tradeoffs" below). The root eval set has a 50% test accuracy due to over-broad keywords like "plan" and "review" — adding negative triggers is recommended.

## Scores by Category
| Category | Score | Notes |
|----------|-------|-------|
| Frontmatter | 25/25 | All 9 skills pass name, description, and format checks. No XML tags. |
| Triggering | 17/20 | Descriptions have WHAT + WHEN + keywords. Root skill eval set scores 50% on test due to false positives from generic terms. |
| Instructions | 24/25 | Highly specific, complete, concrete examples. 3 skills exceed size guideline (accepted by design). |
| Structure | 14/15 | Centralized shared reference (`askuser-format.md`), clean sub-skill hierarchy. Cross-references valid. |
| Scripts | 10/10 | No custom scripts required; uses Claude Code native tools and inline bash. |
| Disclosure | 5/5 | Detailed reference files extracted from 4 review skills. 3-layer system used correctly. |

## Accepted Tradeoffs (Documented)

### Sub-skill Size Exceeding 5000 Tokens

**Decision:** Do not compress `vc-plan-ceo-review` (~12,509 tokens), `vc-plan-devex-review` (~9,062 tokens), or `vc-autoplan` (~8,442 tokens) further.

**Rationale:**

1. **Truncation is compaction-scoped, not initial-load.** The official Claude Code documentation states that invoked skill bodies are "Re-injected, capped at 5,000 tokens per skill" *during context compaction* (source: [Claude Code Docs — Explore the context window](https://code.claude.com/docs/en/context-window)). On initial invocation, the full skill body loads into the active context without truncation. Only if and when a long session compacts is the skill body re-injected and truncated.

2. **"Active working memory" is not a sourced technical term.** No provider (Anthropic, OpenAI, Google, Kimi, Z.ai) documents a concept of skills evaporating from "active working memory" during normal execution. The only documented behavior is truncation on re-injection after compaction.

3. **Further extraction adds execution friction.** These skills are manually invoked (`user-invocable: true`) and rely on sequential phase execution with STOP gates. Extracting their core workflow sections into `references/` would require a `Read` gate before every phase, adding tool-call overhead and breaking the inline flow. The author already extracted all non-sequential reference material (cognitive patterns, prime directives, DX tables, etc.) — what remains is the sequential execution body.

4. **Long sessions are expected, and truncation is acceptable.** These review skills are designed for long, multi-phase conversations. If compaction occurs and truncates the later sections, the model has already processed the early phases (which are at the top of the file — truncation keeps the start). The risk is bounded to losing late-phase instructions in extremely long sessions, which the user accepts.

5. **Already acknowledged in `PLAN.md`.** The build plan explicitly documented this acceptance: *"The author declined further compression. All other validation rules pass cleanly."*

## Critical Issues (fix immediately)
None.

## High Priority (fix within 1 week)
None.

## Medium Priority (nice to have)
- **Root eval set false positives:** The root `vibe-engineering-mastery` eval set has a 0.5 test score because generic words ("plan", "review", "strategic") match too many unrelated prompts. Consider adding negative triggers to the root description, e.g. `Do NOT use for: general project management, daily task tracking, or simple one-line todo items.`
- **Sub-skill eval sets missing:** Only the root skill has an eval set. Each sub-skill (`vc-strategy`, `vc-plan-*-review`, etc.) could have its own `evals/evals.json` for independent trigger testing. The author previously declined eval sets as cosmetic.

## Recommendations
- **Add negative triggers to root description** to narrow scope and reduce false positives.
- **Generate per-sub-skill eval sets** using `scripts/generate_eval_set.py` for each skill directory, if desired.
- **Run `/skill-forge eval`** on the full pack for functional workflow testing (artifact detection, routing gates, Yegge Loop simulation).

## Suggested Test Queries

### Should Trigger
1. "I need to plan this properly before we build — can you run the full pipeline?"
2. "Run reviews before we build this feature."
3. "Let's do a strategic planning session for the new auth system."
4. "Run the orchestrator — I have an important idea that needs stakeholder review."
5. "Start the full review chain: strategy, then CEO, eng, design, and DevEx reviews."
6. "Write our strategy doc and then run all reviews."
7. "I need a product review with scope challenge and architecture lock."
8. "Can you do the Yegge Loop on our implementation plan?"

### Should NOT Trigger
1. "Plan my vacation itinerary for next week." (personal planning, not engineering)
2. "Review this movie for me." (entertainment review)
3. "Write a unit test for the `plan` module in my project." (coding task, not pipeline)
4. "Search GitHub for strategic planning templates." (research, not execution)
5. "Debug why my CI pipeline is failing." (operations, not planning)
6. "Refactor the orchestrator function in my codebase." (code change, not skill invocation)
7. "What's the best approach to learn product management?" (education, not pipeline)
8. "Set up a daily standup agenda." (routine, not strategic planning)

## Validation Results (Per Skill)

| Skill | Score | Status | Issues |
|-------|-------|--------|--------|
| `vibe-engineering-mastery` (root) | **100** | Pass | None |
| `vc-strategy` | **100** | Pass | None |
| `vc-orchestrator` | **100** | Pass | None |
| `vc-writing-plans` | **100** | Pass | None |
| `vc-plan-design-review` | **100** | Pass | None |
| `vc-plan-eng-review` | **95** | Pass | ~7851 tokens (accepted) |
| `vc-plan-ceo-review` | **90** | Pass | 774 lines, ~12509 tokens (accepted) |
| `vc-plan-devex-review` | **90** | Pass | 729 lines, ~9062 tokens (accepted) |
| `vc-autoplan` | **90** | Pass | 734 lines, ~8442 tokens (accepted) |

## Structure Compliance Checklist
- [x] `SKILL.md` exists (exact case) in every skill directory
- [x] No `README.md` inside any skill folder
- [x] Folder names match `name` field in frontmatter
- [x] Valid kebab-case naming (1-64 chars) for all skills
- [x] No "claude" or "anthropic" in any skill name
- [x] `user-invocable: true` present on all 8 sub-skills and root
- [x] Centralized shared reference (`references/askuser-format.md`)
- [x] Sub-skills reference parent shared files via `../../references/`
- [x] `AGENTS.md` documents architecture, conventions, and design philosophy
- [x] `PLAN.md` tracks build status and post-refinement decisions
- [x] `evals/evals.json` exists for root skill

## Architecture Review (Multi-skill)
- [x] Main skill (`vibe-engineering-mastery` + `vc-orchestrator`) has clear routing table
- [x] Sub-skills have focused responsibilities (strategy, CEO review, eng review, design review, DevEx review, autoplan, writing-plans)
- [x] Cross-references are valid (all `references/` files exist and are linked correctly)
- [x] Naming follows `parent-child` convention (`vc-*` prefix for all sub-skills)
- [x] Shared references in parent, not duplicated (centralized `askuser-format.md`)
- [x] Agents have clear roles (Kimi K2.6 for planning/refinement, GLM-5.1 for task breakdown)

## Files Verified
- `vibe-engineering-mastery/SKILL.md`
- `vibe-engineering-mastery/AGENTS.md`
- `vibe-engineering-mastery/PLAN.md`
- `vibe-engineering-mastery/references/askuser-format.md`
- `vibe-engineering-mastery/skills/vc-strategy/SKILL.md`
- `vibe-engineering-mastery/skills/vc-orchestrator/SKILL.md`
- `vibe-engineering-mastery/skills/vc-orchestrator/references/yegge-loop.md`
- `vibe-engineering-mastery/skills/vc-autoplan/SKILL.md`
- `vibe-engineering-mastery/skills/vc-plan-ceo-review/SKILL.md`
- `vibe-engineering-mastery/skills/vc-plan-eng-review/SKILL.md`
- `vibe-engineering-mastery/skills/vc-plan-design-review/SKILL.md`
- `vibe-engineering-mastery/skills/vc-plan-devex-review/SKILL.md`
- `vibe-engineering-mastery/skills/vc-writing-plans/SKILL.md`
- All sub-skill `references/*.md` files

## Action Items
1. Optional: Add negative triggers to root `SKILL.md` description to improve eval test score.
2. Optional: Generate eval sets for each sub-skill directory.
3. Optional: Run functional evaluation via `/skill-forge eval` on the full pack.

## Review Metadata
- **Review date:** 2026-05-13
- **Reviewer:** OpenCode Agent (skill-forge review process)
- **Skills reviewed:** 9 (1 root + 8 sub-skills)
- **Reference:** Anthropic Claude Code Docs — [Explore the context window](https://code.claude.com/docs/en/context-window)
- **Build plan:** `vibe-engineering-mastery/PLAN.md`
