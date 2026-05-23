# Detailed Remediation Plan — kernl-5j6o (DA Core)

**Status:** Awaiting user approval
**Created:** 2026-05-23T23:00:00Z
**Target:** Bring all epic child beads to 900–1000 score

---

## 1. Problem Summary (from Audit Pass 2026-05-23T22-28-10Z)

| Bead | Score | Critical Findings |
|------|------:|-------------------|
| kernl-bypf (U3) | 570 | `NoopLLMClient` returns hardcoded stub in production handlers |
| kernl-zpu0 (U4) | 680 | `stubPermissionChecker` nil-default still exists; deny-path untested |
| kernl-1100 (U6) | 620 | Vanilla JS frontend — **architectural drift** from Vue 3 + Nuxt vision |
| kernl-pf4y (U7) | 720 | Deny-path + concurrency tests missing; some tests use stub |
| **Epic** | 620 | Min child = 620 + theater drag → cannot close |

### What the plan + vision docs confirmed:
1. **LLM integration was NOT deferred.** The plan's "Deferred to Implementation" only lists "timeout of pending permission" and "SSE lifecycle contract." `NoopLLMClient` was never the plan.
2. **Vue 3 + Nuxt IS the target stack.** VISION.md §12 is law. Vanilla JS is architectural drift (Path A: rewrite to Vue now, P2.6 integrates later).

---

## 2. Remediation Beads (4 created)

### 2.1 `kernl-5j6o.1` — LLM Client Integration
**What:** Replace `NoopLLMClient` with real OpenAI, Anthropic, and Ollama providers via a minimal abstraction.
**Est. effort:** Medium (1 developer, 2–3 days)
**Output files:**
- `internal/chat/llm_provider.go`
- `internal/chat/openai_client.go`
- `internal/chat/anthropic_client.go`
- `internal/chat/ollama_client.go`
- `internal/chat/mock_llm.go` (test-only build tag)
- `internal/config/config.go` (add LLM section)

### 2.2 `kernl-5j6o.2` — Permission Engine Cleanup
**What:** Delete `stubPermissionChecker`, make `PermissionChecker` required in `NewChatEngine`, add deny-path integration test, clean stale comments.
**Est. effort:** Small (1 developer, 1 day)
**Output changes:**
- Delete `internal/chat/engine.go:24-28` (stub)
- Modify `internal/chat/engine.go:43` → error on nil pc
- Add `TestChatPermissionDenyAndContinue`
- Add `TestChatPermissionDenyWithFeedback`
- Target: `internal/chat` coverage ≥ 60%

### 2.3 `kernl-5j6o.3` — Vue Frontend Migration
**What:** Rewrite vanilla JS frontend (chat UI, scope selector, DA config) to Vue 3 + Nuxt. Initialize Nuxt, port all features to `.vue` pages/components/composables, add Vitest and Playwright tests.
**Est. effort:** Large (1 developer, 3–5 days)
**Output files:**
- `web/package.json` (Nuxt + Vue + Tailwind + Vitest + Playwright)
- `web/nuxt.config.ts`
- `web/pages/chat.vue`, `web/pages/config/da.vue`
- `web/components/ChatMessageList.vue`, `ChatInput.vue`, `PermissionBanner.vue`
- `web/composables/useChatSession.ts`, `useGraphNodes.ts`
- `vitest` unit tests for all composables + components
- `playwright` E2E tests for chat flow, scope selector, permission prompt
- `web/embed.go` updated for `.output/public/`
- Delete: `web/chat.html`, `web/chat.js`, `web/da-config.html`, `web/da-config.js`, `web/scope-selector.js`

### 2.4 `kernl-5j6o.4` — Integration Test Completion
**What:** Complete the integration test matrix with deny-path, deny-with-feedback, concurrency, and SSE reconnect tests. Remove all `stubPermissionChecker` usage from integration tests.
**Est. effort:** Small (1 developer, 1 day)
**Output changes:**
- Add `TestChatPermissionDenyAndContinue`
- Add `TestChatPermissionDenyWithFeedback`
- Add `TestChatConcurrentMessages`
- Add `TestChatSSEReconnectPreservesState`
- Replace `stubPermissionChecker` in all existing integration tests with real `GraphPermissionChecker`
- Target: 8 scenarios covered by `go test -tags=integration`

---

## 3. Execution Order (Dependencies)

```
kernl-5j6o.1 (LLM) ────╮
kernl-5j6o.2 (Cleanup) ──╯──┐
                            ├──→ kernl-5j6o.4 (Integration Tests)
kernl-5j6o.3 (Vue) ────────╮ (parallel — doesn't block backend tests)
                            │
                            ↓
                    kernl-5j6o (Epic) can close
```

- **kernl-5j6o.1** and **kernl-5j6o.2** can run in **parallel** (they touch different files)
- **kernl-5j6o.4** depends on **kernl-5j6o.2** (needs the deny-path code)
- **kernl-5j6o.3** is independent — backend APIs already exist
- **Epic closes** when all 4 remediation beads plus original 8 child beads score ≥ 900

---

## 4. Scoring Projection (Post-Remediation)

| Bead | Post-remedy score | Why |
|------|-------------------|-----|
| kernl-bypf (U3) | **950** | Real LLM replaces NoopLLMClient — full anti-theater |
| kernl-zpu0 (U4) | **910** | Stubs removed, deny-path tested, coverage ≥ 60% |
| kernl-1100 (U6) | **920** | Vue + automated tests (Vitest + Playwright) |
| kernl-pf4y (U7) | **950** | All 8 scenarios covered, no stubs in tests |
| kernl-ijxz (U1) | 900 (unchanged) | Already good |
| kernl-utd8 (U0) | 935 (unchanged) | Already good |
| kernl-chyw (U2) | 875 (unchanged) | Already good |
| kernl-rjkb (U5) | 920 (unchanged) | Already good |
| **Epic** | **875** (min = U2, then U3/U7) | Above threshold, ship-ready |

---

## 5. Risk Table

| Risk | Impact | Mitigation |
|------|--------|-----------|
| LLM provider library choice causes dependency bloat | Medium | Use stdlib `net/http` for requests; minimal deps only |
| Nuxt build complicates Go embed process | Low | `ssr: false` means static HTML → Go embed is trivial |
| Vue rewrite introduces UI regressions | Medium | Playwright E2E test prevents regressions |
| 60% coverage target on `internal/chat` is hard | Low | Integration tests cover ~50% automatically; unit tests for remainder |
| API key leak in test code | High | Add `.env` to `.gitignore`; never commit keys |

---

## 6. User Approval Checklist

- [ ] **Approve this plan as-is** → I'll assign beads to agents and begin implementation
- [ ] **Modify scope** → Tell me which ACs to change/cut
- [ ] **Request costing/estimates** → I can produce per-bead developer-hour estimates
- [ ] **De-prioritize** → We can defer Vue (Path B: wait for P2.6) and fix only backend stubs first

**Default if no response in 24h: plan auto-approved, implementation begins.**
