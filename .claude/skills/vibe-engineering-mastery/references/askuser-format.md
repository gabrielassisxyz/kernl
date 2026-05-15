# AskUserQuestion Decision Brief Format

> Shared reference for all `vc-plan-*-review` skills. Every AskUserQuestion is a decision brief.

## Tool Resolution

"AskUserQuestion" can resolve to two tools at runtime: the **host MCP variant** (e.g. `mcp__conductor__AskUserQuestion`) or the **native** Claude Code tool.

**Rule:** if any `mcp__*__AskUserQuestion` variant is in your tool list, prefer it. Hosts may disable native AUQ via `--disallowedTools AskUserQuestion` and route through their MCP variant.

**If no AskUserQuestion variant appears in your tool list, this skill is BLOCKED.** Stop, report `BLOCKED — AskUserQuestion unavailable`, and wait for the user.

## Format

Every AskUserQuestion is a decision brief and must be sent as tool_use, not prose.

```
D<N> — <one-line question title>
Project/branch/task: <1 short grounding sentence>
ELI10: <plain English a 16-year-old could follow, 2-4 sentences, name the stakes>
Stakes if we pick wrong: <one sentence on what breaks, what user sees, what's lost>
Recommendation: <choice> because <one-line reason>
Completeness: A=X/10, B=Y/10   (or: Note: options differ in kind, not coverage — no completeness score)
Pros / cons:
A) <option label> (recommended)
  ✅ <pro — concrete, observable, ≥40 chars>
  ❌ <con — honest, ≥40 chars>
B) <option label>
  ✅ <pro>
  ❌ <con>
Net: <one-line synthesis of what you're actually trading off>
```

### Field Rules

- **D-numbering:** first question in a skill invocation is `D1`; increment yourself.
- **ELI10:** always present, in plain English, not function names.
- **Recommendation:** ALWAYS present. Keep the `(recommended)` label.
- **Completeness:** use `N/10` only when options differ in coverage. 10 = complete, 7 = happy path, 3 = shortcut. If options differ in kind, write: `Note: options differ in kind, not coverage — no completeness score.`
- **Pros / cons:** use ✅ and ❌. Minimum 2 pros and 1 con per option when the choice is real; minimum 40 characters per bullet. Hard-stop escape: `✅ No cons — this is a hard-stop choice`.
- **Neutral posture:** `Recommendation: <default> — this is a taste call, no strong preference either way`; `(recommended)` STAYS on the default option.
- **Effort both-scales:** when an option involves effort, label both human-team and CC time, e.g. `(human: ~2 days / CC: ~15 min)`.
- **Net line:** closes the tradeoff.

## Self-Check Before Emitting

Before calling AskUserQuestion, verify:
- [ ] D<N> header present
- [ ] ELI10 paragraph present (stakes line too)
- [ ] Recommendation line present with concrete reason
- [ ] Completeness scored (coverage) OR kind-note present (kind)
- [ ] Every option has ≥2 ✅ and ≥1 ❌, each ≥40 chars (or hard-stop escape)
- [ ] (recommended) label on one option (even for neutral-posture)
- [ ] Net line closes the decision
- [ ] You are calling the tool, not writing prose

## Non-ASCII Characters

Write directly, never \u-escape. When any string field contains Chinese, Japanese, Korean, or other non-ASCII text, emit literal UTF-8. Claude Code's tool pipe is UTF-8 native.

Only JSON-mandatory escapes allowed: `\n`, `\t`, `\"`, `\\`.
