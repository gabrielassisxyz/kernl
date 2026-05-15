# Engineering Review — Workflow do Kernl + MergeManager + Sweep

**Data:** 2026-05-15
**Spec revisada:** [`docs/2026-05-15-kernl-workflow-brainstorm-spec.md`](../2026-05-15-kernl-workflow-brainstorm-spec.md)
**Skill:** `vibe-engineering-mastery/skills/vc-plan-eng-review`
**Reviewer:** opus-4-7 + outside voice (subagent independente)

---

## TL;DR

A spec é arquiteturalmente sólida — 3-camada de persistência espelhando gastown é cargo cult deliberado e correto; reuso 100% do take loop/watchdog/dispatch é a escolha certa pro custo. O review identificou 14 issues materiais (5 architecture, 3 code quality, 3 test, 3 performance) e o outside voice levantou 5 cross-model tensions; todas foram resolvidas com decisões registradas (D1–D14, TT1–TT5).

**Escopo final do PR:** workflow migration + MergeManager + sweep. Knots delete fica como follow-up PR (mantém decisão da eng review anterior).

**Mudança material relativa à spec original:** AgentState **sai do bd.description** e vai pra JSON local em `~/.kernl/state/<bead-id>.json` (D1=C). É a única divergência conceitual em relação ao modelo do gastown — motivada por race de escritor concorrente que gastown evitava por arquitetura single-mux-server, ausente no kernl.

**Blocker pré-implementação:** TT1=A exige um spike empírico (~1-2h) medindo taxa de conflito em 3 epics fechadas históricas. Se >30%, design do MergeManager precisa pivotar de batch-no-fim para incremental-conforme-filho-termina antes de qualquer código ser escrito.

---

## Step 0 — Scope Challenge

**Complexity check: TRIGGERED.** Spec original empilhava 3 movimentos no mesmo big-bang PR: (1) workflow migration, (2) MergeManager + auto-merger, (3) knots delete completo. Soma ~880 LOC novos + 27 arquivos refatorados.

**Decisão D0 — Scope:** **C — workflow + MergeManager/sweep no PR; knots delete vira follow-up.**

Razão: merger/sweep custam ~880 LOC sobre infra pronta (take loop/watchdog) e o argumento da spec (D4 do MVP anterior era equivalente a "humano abre sessão manual e manda mergear") é honesto. Knots delete é refactor próprio de 14 arquivos, sem payoff funcional pro MVP, e contradiz a decisão anterior da eng review de manter knots dormante. Respeita o princípio "explicit > clever — evitar acoplar movimentos não-relacionados num PR só."

---

## Section 1 — Architecture Review

### D1 — AgentState fora do bd.description, vai pra JSON local

**Issue:** `bd.description` é campo único de texto. bd CLI 1.0.4 não tem update parcial — toda escrita é substituição total. Em épico com filhos paralelos, worker A faz heartbeat enquanto worker B (outro bead, mas merger pode escrever no épico-pai concorrentemente) — last-write-wins → lost-update → watchdog falso positivo.

`[P1] (confidence: 8/10) workflow/description.go (proposto) — race condition concorrente`

**Decisão: C** — `AgentState` (heartbeat, follow_up_count, watchdog state, agent_session_id, agent_started_at) vive em JSON local `~/.kernl/state/<bead-id>.json`. `bd.description` mantém apenas campos estáveis: `worktree_path`, `worktree_branch`, `epic_branch`, `pr_url`, `merge_conflict_at`, `merge_outcome`. Esses só são escritos 1-2x por bead durante o lifecycle inteiro.

**Trade-off aceito:** diverge do gastown na taxonomia das camadas (gastown coloca tudo em description e serializa via single-mux-server). Kernl é multiprocess — divergir aqui é a escolha consciente.

### D2 — Source of truth: bd autoritativo, JSON efêmero

**Issue:** Com D1=C, transição "agent done" envolve dois writes: (1) `agent_state: "done"` no JSON, (2) `bd update --status awaiting_integration`. Crash entre os dois deixa estado divergente.

`[P1] (confidence: 9/10) merge/manager.go (proposto) trigger logic`

**Decisão: A** — bd é a única SoT pra trigger logic. MergeManager/EpicExecutor consultam SEMPRE `bd list --status=awaiting_integration` parent-filtrado. JSON local guarda só dados de runtime (heartbeat, contadores, watchdog) que podem ser reset no boot sem perda funcional. Ordem canônica de write: bd primeiro, file segundo.

```
TRANSITION (agent reports done)
    ↓
    [1] bd update <id> --status awaiting_integration   ← SoT, atomic do ponto de vista do trigger
    [2] write JSON file with agent_state: done         ← observability/audit, perda OK em crash
```

### D3 — Failure modes do merger enumerados + transitions explícitas no MergeManager

**Issue:** Spec §5.4 só trata "conflito não-resolvido → blocked". Não cobre `git push` failed (auth, rede, conflito remoto) nem `gh pr create` failed após push OK (PR já existe, rate limit, perms). Resultado: branch pushada sem PR → épico fica órfão em `in_progress`.

`[P1] (confidence: 9/10) merge/manager.go + prompt/merger_prompt.go (propostos)`

**Decisão: A** — adicionar ao prompt do merger:
- Se `git push` falha → retry 2x com backoff exponencial, então escreve `merge_outcome: push_failed` na description e termina.
- Se `gh pr create` falha — detecta "PR já existe" (busca pr_url existente e adopta), demais erros → `merge_outcome: pr_create_failed`.

MergeManager lê `merge_outcome` da description após agent done e roteia transições determinísticamente. PR pré-existente → `awaiting_pr_review` (idempotent re-run); demais failures → `blocked` com contexto. (Ver D6 pro enum tipado.)

### D4 — `cmd/kernl/epic_merge.go` entra no escopo do PR

**Issue:** §5.4 promete subcomando `kernl epic merge <epic-id>` (re-disparar passo de integração após conflito resolvido manualmente), e §9 critério #8 confirma a promessa, mas §8.2 "Novos" não lista o arquivo.

`[P0] (confidence: 10/10) spec inconsistência direta`

**Decisão: A** — `cmd/kernl/epic_merge.go` adicionado ao escopo. Implementação: valida épico em `blocked` com `merge_conflict_at`, valida filhos em `awaiting_integration` ou `closed`, marca épico `in_progress`, dispara MergeManager. Idempotente — re-rodar em estado errado retorna erro descritivo.

### D5 — Sweep auto-tick: skip-on-fail + circuit breaker + cache MERGED

**Issue:** Auto-tick em `kernl serve` (60s default) faz `gh pr view` por épico em `awaiting_pr_review`. Sem cache, sem circuit breaker, sem política pra falha do `gh` (rede caída, token expirado).

`[P2] (confidence: 8/10) sweep/sweep.go (proposto) network resilience`

**Decisão: C** — três camadas:
1. **Skip-on-fail:** cada `gh pr view` que falha vira log WARN com `(epic_id, err)`; tick continua com demais.
2. **Circuit breaker:** per-bead counter em memória; 3 falhas consecutivas → backoff exponencial (5min, 15min, 60min). Reset ao sucesso.
3. **Cache MERGED:** uma vez detectado MERGED, `map[prURL]bool` evita re-consulta (GitHub não desfaz merge).

```
SWEEP TICK FLOW
  for epic in bd list --status=awaiting_pr_review:
    if cache[epic.pr_url] == MERGED:
      → bd close (idempotent)
      continue
    if breaker.open(epic.id):
      → skip
      continue
    state = gh pr view ...
    on error:
      breaker.fail(epic.id)
      WARN log
      continue
    if state == MERGED:
      cache[epic.pr_url] = true
      → bd close
```

---

## Section 2 — Code Quality Review

### D6 — Enum tipado `merge.Outcome` + description field `merge_outcome:`

**Issue:** D3=A diz "MergeManager roteia baseado em failure mode". Sem tipagem, vira string-matching no `error.Error()` — frágil ao wording do prompt.

`[P2] (confidence: 8/10) merge/errors.go (proposto)`

**Decisão: A** — vocabulário compartilhado:

```go
// merge/errors.go
package merge

type Outcome string

const (
    OutcomeSuccess         Outcome = "success"
    OutcomeMergeConflict   Outcome = "merge_conflict"
    OutcomePushFailed      Outcome = "push_failed"
    OutcomePRCreateFailed  Outcome = "pr_create_failed"
    OutcomePRAlreadyExists Outcome = "pr_already_exists"
)

func ParseOutcome(s string) (Outcome, error) { /* ... */ }
```

Prompt do merger inclui literal essa lista. MergeManager parseia via switch exaustivo (compilador pega outcome novo não-tratado).

### D7 — Cargo cult: EnsureCustomStatuses chamado em toda operação (cópia gastown 1:1)

**Issue:** Spec propõe espelhar gastown em "chamar em toda operação". Em kernl single-binary, isso pode ser desperdício.

`[P3] (confidence: 7/10) workflow/ensure_custom.go (proposto) hot path overhead`

**Decisão: B** — manter cargo cult 1:1 (cache em memória + sentinel + chamada em toda op).

Razão: cache em memória elimina o overhead real após primeira call; sentinel file em `.beads/` detecta staleness se outro processo mexer no `bd config`. Coerente com a tese §3.4 da spec ("gastown rodou em produção, mesmo problema; cargo cult deliberado"). Code smell estético sem custo prático real.

**Confirmado novamente em TT3=B** (apesar do challenge do outside voice de pivotar pra bootstrap-once).

### D8 — Naming determinístico: `feat/<epic-id>` puro

**Issue:** Spec §5.2 step 1 diz `feat/<epic-title-or-id>` — ambíguo. Slug de title tem edge cases (acentos, emoji, length, rename desincroniza branch).

`[P3] (confidence: 9/10) spec ambigüidade`

**Decisão: A** — branch sempre `feat/<epic-id>` (deterministic, unique, short). PR title leva o nome legível via `gh pr create --title <epic.title>` no merger.

---

## Section 3 — Test Review

### Test Framework Detection

Go padrão. `*_test.go` + `-tags=integration` pra integration suite. Harness em `orchestrator/internal/integration/harness.go` usa `bd init --from-jsonl`. 962 unit tests existentes usam fakes que aceitam qualquer string de status — por isso a quebra contra bd 1.0.4 real só aparece em integration.

### Coverage Diagram (do escopo desta spec)

```
CODE PATHS                                                                          STATUS
[+] workflow/status.go
  ├── IssueStatus predicates (4 methods × 6 valores + unknown/empty)               [unit GAP]
  ├── KernlCustomStatuses slice (assertion sobre conteúdo exato)                  [unit GAP]
  └── IsValidCombination(IssueStatus × AgentState)  ← novo per TT5=A               [unit GAP]

[+] workflow/description.go (encolhido por D1=C: só campos estáveis)
  ├── getMetadataField — case-insens, valor com `:`, BOM, vazio, multilinha       [unit GAP]
  └── addMetadataField — insert, update, preservar outras linhas                  [unit GAP]

[+] workflow/agent_state_store.go  (NOVO per D1=C)
  ├── Read missing file → defaults                                                [unit GAP]
  ├── Read corrupted JSON → recover com defaults + WARN (D9=A)                    [unit GAP CRÍTICO]
  ├── Atomic write (tempfile + rename per D12=A)                                  [unit GAP]
  ├── Mutex em-processo                                                            [unit GAP]
  └── Purge on bead close                                                          [→E2E GAP]

[+] workflow/ensure_custom.go  (D7=B cargo cult gastown)
  ├── Fresh bd → register both                                                    [unit GAP]
  ├── Customs já set → no-op
  ├── Merge com customs estrangeiros
  ├── bd config falha → erro propagado fail-loud
  └── Sentinel staleness → re-validate

[+] merge/errors.go  (NOVO per D6=A)
  └── Outcome enum + ParseOutcome — trivial                                       [unit GAP leve]

[+] merge/manager.go
  ├── Trigger detect: parent ∧ todos filhos awaiting_integration via single bd list (D14)
  │   └── Single-flight lock per epic_id (D11=A)                                  [unit GAP]
  │   └── Determínistic concurrent test (N goroutines tentando trigger)         [unit GAP]
  ├── Parse merge_outcome from description (D6=A) → switch exaustivo              [unit GAP]
  ├── Outcome ausênte/inválido → blocked com diagnóstico                       [unit GAP]
  └── Dispatch merger agent (reusa take loop infra)                               [→E2E GAP]

[+] prompt/merger_prompt.go
  └── Render N=1, N=3, N=10 children; com/sem conflict; lista de outcomes literal [golden GAP]

[+] sweep/sweep.go  (D5=C)
  ├── Happy: PR MERGED → bd close filhos + epic                                   [unit GAP]
  ├── Cache hit no 2º tick (PR MERGED já visto)                                [unit GAP]
  ├── gh fails 3× → breaker open → backoff 5/15/60min                             [unit GAP]
  ├── Breaker half-open: 1 sucesso → reset                                        [unit GAP]
  ├── PR stale WARN (>N dias) per TT2=B                                           [unit GAP]
  └── --dry-run: zero writes, output formatado                                    [unit GAP]

[+] cmd/kernl/sweep.go                                                            [unit GAP leve]
[+] cmd/kernl/epic_merge.go  (NOVO per D4=A)
  ├── Valida épico blocked com merge_conflict_at                                  [unit GAP]
  ├── Valida filhos awaiting_integration ou closed                                [unit GAP]
  ├── Re-dispatch idempotente                                                     [unit GAP]
  └── Erro: estado inconsistente → mensagem clara                                 [unit GAP]

[+] EpicExecutor (modificado)
  ├── Single-flight trigger "all children awaiting_integration"                   [unit + →E2E]
  ├── Child blocked → halt epic, nÃO disparar merger                            [unit GAP]
  └── Race: último filho transiciona durante scan                                [→E2E GAP]

USER FLOWS  (todos GAP, todos críticos)
  ├── [→E2E] Happy: 3 filhos paralelos → merger → PR aberto → sweep fecha
  ├── [→E2E] Conflict: filhos OK → conflito → blocked → kernl epic merge → success
  └── [→E2E] Push fail: branch pushada, gh pr create falha → blocked → recover

REGRESSION SUITE
  ├── [CRITICAL] sed map fixtures: ready_for_implementation→open, etc. (per spec §8.4)
  ├── [CRITICAL] 27 arquivos *_test.go com string literals legacy → constantes de workflow/
  └── [→E2E] Passo A integration test verde contra bd 1.0.4 real ← critério #1 da spec

COVERAGE: 0/26 paths testados (spec ainda não implementada)
GAPS: 26 (3 E2E críticos, ~22 unit, 1 regression-suite)
```

### D9 — JSON corrompido → recover com defaults + WARN

**Issue:** D1=C cria store JSON local. Cenários: truncated (mitigado por D12=A atomic write), schema mudou (versão futura), arquivo deletado, permissions.

`[P2] (confidence: 9/10) workflow/agent_state_store.go (proposto)`

**Decisão: A** — recover com defaults + log WARN. Coerente com D2=A (bd autoritativo). Usuário pode `rm -rf ~/.kernl/state/` pra reset sem brick.

### D10 — Pinagem explícita de bd CLI ≥ 1.0.4 no harness + CI

**Issue:** Integration tests precisam bd 1.0.4+ (versões antigas aceitam status legacy ou validam diferente). Harness não checa versão; CI pode rodar com bd antigo e quebrar em prod.

`[P1] (confidence: 8/10) integration/harness.go`

**Decisão: A** — `harness.go` valida `bd --version >= 1.0.4` no SetUp com fail-fast; CI installs versão pinada via `go install bd@v1.0.4`. Versão mínima também documentada em `AGENTS.md` e README.

### D11 — Single-flight lock + deterministic test pro trigger

**Issue:** EpicExecutor detecta "todos filhos awaiting_integration" e dispara merger. Race: duas detecções simultâneas → dois mergers competindo na mesma epic branch.

`[P1] (confidence: 7/10) merge/manager.go (proposto) concurrency`

**Decisão: A** — `sync.Map[epic_id]chan struct{}` como single-flight lock no MergeManager + test determinístico com N goroutines tentando trigger simultaneamente (sync.WaitGroup, sem timing aleatório). Property-based test fica como TODO (T1).

---

## Section 4 — Performance Review

### D12 — Atomic write tempfile+rename pro agent state JSON

**Issue:** Heartbeat ~1ms/op em SSD. Em HDD ou NFS pode crescer.

`[P3] (confidence: 7/10) workflow/agent_state_store.go`

**Decisão: A** — manter atomic write como correctness baseline. Adicionar config `agent_heartbeat_interval` em `kernl.yaml` (default 10s). Batch heartbeats em memória fica como TODO (T2).

### D13 — Sweep auto-tick 60s configurável; skip empty logs

**Issue:** Tick custa `bd list --status=awaiting_pr_review` (~30-50ms fork+exec) mesmo em idle.

`[P3] (confidence: 6/10) sweep/sweep.go`

**Decisão: A** — manter 60s default; configurável via `sweep.auto_interval_seconds`. Skip logging quando `len(epics) == 0` pra não poluir log de longa rodagem.

### D14 — Single `bd list` query pro check "all children done"

**Issue:** Iterar `bd show <child-id>` × N seria N+1.

`[P3] (confidence: 6/10) merge/manager.go`

**Decisão: A** — usar `bd list --status=awaiting_integration --parent=<epic-id> --json` + comparação de contagem em-processo. Constant-cost independente de N filhos. Fallback se `--parent` não existir: query global + filtro Go-side.

---

## Outside Voice — Cross-Model Tensions

### TT1 — Conflict rate empirical validation pré-código

**Tension:** review interno aceitou "merger tenta resolver" sem medir. Outside voice argumenta: se taxa de conflito em epics reais >30%, design batch-no-fim quebra → precisa pivotar pra incremental-conforme-filho-termina.

**Decisão: A** — **rodar spike experimental antes de qualquer código:**

1. Selecionar 3 epics fechadas históricas (commits de filhos identificáveis).
2. Em worktree limpo, criar branch base no commit pré-epic.
3. Merge sequencial topológico dos commits-filho.
4. Medir taxa de conflito (% de merges que requerem manual resolution).
5. Se >30%: re-shape do design para incremental-merge (cada filho mergeia na epic branch ao terminar, não em batch no fim).

**Custo:** 1-2h. **Valor:** evita reescrever o MergeManager pós-MVP se a premissa quebrar. **BLOCKER pré-implementação.**

### TT2 — PR staleness WARN (sem ação automática)

**Tension:** outside voice: `awaiting_pr_review` é estado vivo num sistema que pode estar morto.

**Decisão: B** — `kernl sweep` adiciona warning "PR aberto há N dias" (N configurável, default 7) sem ação automática. Usuário decide.

```yaml
# kernl.yaml
sweep:
  pr_stale_warn_days: 7   # 0 = desabilitado
```

### TT3 — Manter D7=B (cargo cult gastown EnsureCustomStatuses)

**Tension:** outside voice argumenta "1:1 com gastown virou justificativa pra não pensar; bootstrap-once via kernl doctor seria mais limpo."

**Decisão: B (manter D7=B).** Cache em memória elimina overhead real. Spec §3.4 tese mantida. Code smell estético sem custo prático. Reverter D7 introduziria risco de subcomando esquecer de chamar bootstrap.

### TT4 — `kernl epic abort` vira TODO

**Tension:** outside voice: "operador querendo abortar épico em andamento não tem caminho limpo."

**Decisão: B** — TODO pós-MVP. No MVP, abort manual via `bd close --reason="aborted"` por bead + cleanup manual de worktrees. Feio mas funcional. Adiciona-se em `TODOS.md` como T3.

### TT5 — Tabela `(IssueStatus × AgentState) → válido`

**Tension:** outside voice: "6 estados é simplificação aparente — espaço efetivo é o produto cartesiano."

**Decisão: A** — adicionar tabela documentacional em `§4` da spec amendada + função `IsValidCombination(IssueStatus, AgentState) bool` em `workflow/status.go` com test exaustivo. Combinações inválidas explícitas (ex: `closed × working` = bug detectável).

```
                  | spawning | working | done | stuck | failed |
------------------|----------|---------|------|-------|--------|
open              |   N      |   N     |  N   |   N   |   N    | (sem agent ainda)
in_progress       |   Y      |   Y     |  Y   |   Y   |   Y    | (todas válidas, mid-life)
awaiting_integration|  N     |   N     |  Y*  |   N   |   N    | (* só audit trail post-done)
awaiting_pr_review  |  N     |   N     |  Y*  |   N   |   N    | (* só audit trail post-merger)
blocked           |   N      |   N     |  N   |   Y   |   Y    | (motivo do bloqueio fica registrado)
closed            |   N      |   N     |  Y*  |   N   |   N    | (* histórico, não muda mais)
```

`IsValidCombination` retorna false pras células marcadas N. Asserts em writes detectam regressão de runtime.

---

## NOT in scope (deferido com rationale)

- **Knots delete completo** — D0=C, vai pra follow-up PR após estabilização do workflow novo. Mantém decisão da eng review anterior. (TODO existente.)
- **Resolution agent especializado em conflitos** (Self-Healing Merges) — §10 da spec.
- **AST-aware parallelism detection** — §10.
- **GitHub webhook pra sweep** — §10. Polling com circuit breaker (D5=C) resolve no MVP.
- **Perfis customizáveis** (autopilot/semiauto/etc.) — §10, W9.
- **Estados de gate humano por bead** (`awaiting-gate`, etc.) — §10.
- **`epic_post_merge_command`** (testes pós-merge antes do PR) — §10, W5/§5.5.
- **Limpeza automática de worktrees** — TODOS.md existente.
- **`schema_version` no JSON do agent_state** — D9=A escolheu recover puro; versionamento posterga.
- **Property-based race test pro trigger** — D11B → TODO T1.
- **Batch heartbeats em memória** — D12B → TODO T2.
- **`kernl epic abort`** — TT4=B → TODO T3.
- **PR staleness TTL com ação automática** — TT2=B foi WARN-only no MVP; ação automática (auto-blocked após N dias) fica eventual TODO se padrão de uso justificar.

---

## What already exists (reuso vs reconstrução)

| Subproblema | Reusa | Spec captura corretamente? |
|---|---|---|
| Take loop / watchdog / follow-up cap | foolery-go portado (`terminal/`) — 100% reuso | ✓ §5.8 |
| Dispatch de agent | `dispatch/` portado — 100% reuso | ✓ §5.8 |
| SSE / SessionConnectionManager | 100% reuso | ✓ §5.8 |
| `bd status.custom` | bd 1.0+ built-in (Camada 2) | ✓ §3.2 |
| Padrão de description fields | gastown `integration.go:69-128` referência canônica | ✓ §3.3 (mas reduzido a campos estáveis por D1=C) |
| EnsureCustomStatuses idempotente | gastown `beads_types.go:187` (D7=B copia 1:1) | ✓ §3.2 |
| WorktreeManager (criar epic branch + children) | adiciona ~50 LOC sobre o existente | ✓ §5.7 |
| `gh pr view --json state,mergedAt` | gh CLI built-in (Layer 1) | ✓ §6.2 |
| `bd close` em batch | bd CLI built-in | ✓ §6.2 |
| **AgentState em description** (originalmente proposto) | **NÃO reusa gastown 1:1** — vai pra JSON local (D1=C) | ⚠ spec precisa ser amended |

---

## Failure modes table

| Codepath | Failure realista | Test cobre? | Error handling? | Usuário vê? |
|---|---|---|---|---|
| `agent_state_store.go` write | Disk full | GAP — adicionar | Sim, propaga | Log WARN no heartbeat |
| `agent_state_store.go` read | JSON corrompido | D9=A coverage planejada | Sim, recover | Log WARN |
| `ensure_custom.go` | bd config falha | GAP | Fail-loud no startup | Sim, kernl não sobe |
| `merge/manager.go` trigger duplicate | Race entre detecções | D11=A coverage planejada | Single-flight | N/A — mitigado |
| `merge/manager.go` outcome parse | Outcome inválido/ausente | D6=A coverage planejada | → blocked com diag | Sim, motivado |
| `merger_prompt` N=0 | Épico vazio | GAP — smoke test | Erro em render | Sim, degenerate |
| merger → git push | Auth/rede/conflict remoto | D3=A retry+blocked | → `merge_outcome: push_failed` | Sim |
| merger → gh pr create | PR já existe | D3=A detect & adopt | → `pr_already_exists` adopta | N/A — silent OK |
| `sweep/sweep.go` gh fail | Rede caída | D5=C circuit breaker | Skip + backoff | Log WARN; outage longo → silencia |
| `cmd/kernl/epic_merge.go` | Estado errado | GAP | Erro descritivo | Sim |

**Critical gaps (sem teste E sem error handling E silent):** zero.

---

## Worktree parallelization strategy (pra implementação)

```
ONDA 1 (paralela):
  Lane A: workflow/ (status.go, description.go, agent_state_store.go, ensure_custom.go)
  Lane G: specs/ (00-architecture.md, backend/, orchestration/, prompt/)

ONDA 2 (paralela após A merge):
  Lane B: sweep/ + cmd/kernl/sweep.go
  Lane C: backend/bdcli.go refactor + EnsureCustomStatuses wiring
  Lane D: refactor string literals legacy em dispatch/, orchestration/, terminal/, retake/, epic/, app/ + tests
  Lane E: fixtures (testdata/beads-*/.beads/issues.jsonl) + harness bd 1.0.4 pin
  Lane F1: merge/ (errors.go, manager.go)

ONDA 3 (após F1):
  Lane F2: prompt/merger_prompt.go + cmd/kernl/epic_merge.go + EpicExecutor wiring

MERGE ORDER: G → A → C/D/E → F1 → B → F2
```

**Conflict flag:** Lanes D e E ambos tocam `*_test.go` em `internal/integration/` — execução sequencial entre eles ou coordenação manual em pre-flight.

---

## Required Actions (antes da implementação)

1. **TT1 spike experimental** (BLOCKER pré-código): medir taxa de conflito em 3 epics fechadas históricas. Se >30%, re-shape do MergeManager pra incremental-merge.
2. **Amend brainstorm-spec** (`docs/2026-05-15-kernl-workflow-brainstorm-spec.md`):
   - §3.3 + §4.4 + §4.5: AgentState **migra pra JSON local** `~/.kernl/state/<bead-id>.json`; description mantém só campos estáveis.
   - §4 (novo): tabela `(IssueStatus × AgentState) → válido` (TT5=A).
   - §5.3: prompt do merger enumera 3 failure modes + lista literal dos outcomes do enum.
   - §5.4: política de conflito reescrita com transições explícitas baseadas em `merge_outcome`.
   - §5.7: adicionar `cmd/kernl/epic_merge.go` à tabela de componentes novos.
   - §6.3: adicionar `pr_stale_warn_days` ao yaml example; adicionar circuit breaker config.
   - §6 (novo): cache MERGED + circuit breaker behavior.
   - §8.2 "Novos": adicionar `cmd/kernl/epic_merge.go`, `workflow/agent_state_store.go`, `merge/errors.go`.
   - §8.3 gates: adicionar pinagem `bd >= 1.0.4` no harness + CI.
3. **TODOS.md updates:** T1 (property race test), T2 (batch heartbeats), T3 (`kernl epic abort`).

---

## Completion Summary

- **Step 0 (Scope Challenge):** scope reduzido — knots delete diferido pra follow-up (D0=C)
- **Architecture Review:** 5 issues resolvidas (D1-D5)
- **Code Quality Review:** 3 issues resolvidas (D6-D8)
- **Test Review:** coverage diagram produzido, 26 gaps identificados, D9-D11 resolvidas
- **Performance Review:** 3 issues resolvidas (D12-D14)
- **NOT in scope:** escrito (13 itens deferidos com rationale)
- **What already exists:** escrito (10 reuses identificados, 1 divergência consciente)
- **TODOS.md updates:** 3 itens propostos e confirmados (T1, T2, T3)
- **Failure modes:** 10 codepaths analisados, 0 critical gaps
- **Outside voice:** rodado (subagent independente), 5 cross-model tensions, todas resolvidas (TT1-TT5)
- **Parallelization:** 7 lanes em 3 ondas, 1 conflict flag (Lanes D/E em integration tests)
- **Lake Score:** 13/14 recommendations escolheram a opção completa (única exceção D7=B que mantém cargo cult deliberado por design)

**Blocker:** TT1 spike empírico antes da implementação começar.

**Próximo passo:** amend da brainstorm-spec com as decisões listadas em "Required Actions", depois `vc-writing-plans` pra produzir o plano de implementação.
