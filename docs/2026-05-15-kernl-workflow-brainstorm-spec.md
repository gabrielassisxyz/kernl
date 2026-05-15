# Brainstorm Spec — Workflow do Kernl

**Data:** 2026-05-15
**Escopo:** Definição do workflow próprio do kernl (substitui a state machine herdada do foolery), arquitetura de persistência espelhando o gastown, MergeManager + agente de integração automática, subcomando `kernl sweep`, e plano de migração big-bang.
**Status:** design aprovado + amended pós-eng-review 2026-05-15 — pronto para `vc-writing-plans`.
**Referências cruzadas:**
- TODO "Definir workflow próprio do kernl (substituir state machine herdada do foolery)" em `TODOS.md`
- Brainstorm-spec do MVP do núcleo: `docs/2026-05-14-orchestration-nucleo-mvp-brainstorm-spec.md`
- Eng review desta spec: `docs/reviews/vc-plan-eng-review-2026-05-15.md` (14 decisões D1–D14 + 5 cross-model tensions TT1–TT5)
- Test plan: `docs/reviews/vc-plan-eng-review-test-plan-2026-05-15.md`
- Eng review anterior (MVP do núcleo): `docs/reviews/vc-plan-eng-review-2026-05-14.md`
- Strategy: `docs/STRATEGY.md`
- Gastown como referência arquitetural: `/home/gabriel/repositories/_cloned/gastown/internal/beads/status.go`, `internal/beads/integration.go`, `internal/beads/beads_types.go`, `internal/constants/constants.go`

**Amendments aplicados (2026-05-15, pós-review):**
- §3.3 + §4.4 + §4.5 — AgentState **migra pra JSON local** em `~/.kernl/state/<bead-id>.json`; description guarda só campos estáveis (D1=C). Única divergência consciente do modelo gastown.
- §4 — adicionada tabela `(IssueStatus × AgentState) → válido` + função `IsValidCombination` (TT5=A).
- §5.3 — prompt do merger enumera 3 failure modes explicitamente + lista literal dos outcomes do enum tipado (D3=A, D6=A).
- §5.4 — política de conflito reescrita com transições explícitas baseadas em `merge_outcome` (D3=A).
- §5.7 — `cmd/kernl/epic_merge.go` adicionado à tabela de componentes novos (D4=A).
- §6 + §6.3 — cache MERGED + circuit breaker + PR stale WARN no sweep (D5=C, TT2=B).
- §7 (Decisões) — W12, W13, W14 adicionadas refletindo as decisões da eng review.
- §8.2 "Novos" — `cmd/kernl/epic_merge.go`, `workflow/agent_state_store.go`, `merge/errors.go` listados.
- §8.3 — pinagem `bd >= 1.0.4` no harness + CI adicionada como gate (D10=A).
- §10 — entradas novas: property race test, batch heartbeats, `kernl epic abort` (T1, T2, T3 em TODOS.md).
- §11 — próximo passo é `vc-writing-plans`; spike empírico (TT1) **já executado** (resultado: taxa de conflito 0% em 9 merges válidos — design batch valida).

---

## 1. Resumo em um parágrafo

O kernl herdou do foolery uma state machine de 13 estados (`ready_for_planning` → … → `shipped`) modulada por 6 perfis (`autopilot`, `semiauto`, etc.). Esse vocabulário era escrito direto no campo `status` do bd, que era permissivo até a versão 1.0.4 — agora o bd valida `status` contra um conjunto restrito (`open`, `in_progress`, `blocked`, `closed`, etc.). Resultado: o orquestrador está code-complete mas não roda end-to-end contra bd real (962 unit tests passam porque usam fakes; integration tests quebram com `validation failed: invalid status: ready_for_implementation`). Esta spec define um workflow novo, próprio do kernl, espelhando a arquitetura do **gastown** (que resolveu exatamente este problema): três camadas de estado — built-in do bd, custom registrado via `bd config set status.custom`, e structured fields embutidos na description text do bead. O escopo inclui também o **MergeManager com agente de integração automática** (substitui o D4 "review/merge manual" do spec MVP anterior) e o subcomando **`kernl sweep`** para fechar beads cujos PRs já foram mergeados na master, eliminando o `bd close` manual.

---

## 2. Problema concreto

Hoje, em `orchestrator/internal/backend/bdcli.go:208-241`:

```go
func (b *BdCliBackend) Update(id string, input UpdateBeadInput, repoPath string) error {
    args := withRepo(repoPath, "update", id, "--json")
    // ...
    if input.State != "" {
        args = append(args, "--status", input.State)
    }
    // ...
}
```

Onde `input.State` carrega strings como `"ready_for_implementation"`, `"implementation_review"`, `"shipment_review"`, etc. — vocabulário inteiro vindo de `state_machine.go:139-160`. bd 1.0.4 rejeita.

A camada de fakes nos 962 unit tests aceita qualquer string, então o problema só aparece quando `BdCliBackend` executa contra um bd CLI real — exatamente o que o Passo A do MVP exige.

---

## 3. Arquitetura de persistência (espelho do gastown)

Três camadas ortogonais:

### 3.1 Camada 1 — `bd status` built-in (validado, pequeno, estável)

Conjunto fixo aceito pelo bd 1.0.4 sem configuração: `open`, `in_progress`, `blocked`, `closed`, `tombstone`, `pinned`, `hooked`. O kernl usa apenas o subconjunto que faz sentido pro workflow.

### 3.2 Camada 2 — `bd status.custom` (extensão por config)

bd 1.0+ tem o config knob:

```bash
bd config set status.custom "awaiting_integration,awaiting_pr_review"
```

Após esse set, bd valida e aceita esses valores em `bd update --status X`. Gastown chama isso via `EnsureCustomStatuses(beadsDir)` (referência: `gastown/internal/beads/beads_types.go:187`) idempotentemente em toda operação, com **cache em memória + sentinel file em disco** pra detecção de staleness. **Faz merge** com customs existentes (preserva o que outros consumidores registraram). O kernl implementa o mesmo padrão.

### 3.3 Camada 3 — Structured fields em `bd.description` (texto livre)

**[AMENDED 2026-05-15 — D1=C]** Originalmente esta camada cobria TODO o estado de runtime (incluindo `AgentState`). Pós-eng-review, ela cobre apenas **campos estáveis** — escritos 1-2× por bead durante o lifecycle inteiro:

| Field | Onde | Quando muda |
|---|---|---|
| `worktree_path` | filho | uma vez ao spawnar worktree |
| `worktree_branch` | filho | idem |
| `epic_branch` | épico | uma vez na criação do épico |
| `pr_url` | épico | uma vez após `gh pr create` bem-sucedido |
| `merge_conflict_at` | épico | uma vez se merger detectou conflito |
| `merge_outcome` | épico | uma vez quando merger termina (enum: success/merge_conflict/push_failed/pr_create_failed/pr_already_exists — ver §5.3) |

**`AgentState` runtime (state que muda alto-frequência — heartbeat, follow_up_count, watchdog state, agent_session_id, agent_started_at) NÃO vive em description.** Vai pra JSON local em `~/.kernl/state/<bead-id>.json` (ver §4.4). Razão: `bd.description` é único campo de texto sem update parcial — bd CLI faz substituição total. Em épico com filhos paralelos, escritas concorrentes (worker heartbeat + merger update do épico-pai) gerariam lost-update → watchdog falso positivo. Gastown evita pelo single-mux-server; kernl é multiprocess, então diverge aqui conscientemente.

Padrão de parsing/writing pros campos estáveis espelhando `gastown/internal/beads/integration.go:69-128`:

```go
// getMetadataField extrai "key: value" da description (case-insensitive na key).
// addMetadataField insere/atualiza idempotentemente.
```

Tipos Go separados pra cada conceito embutido. `workflow/description.go` mantém `getMetadataField` / `addMetadataField` + helpers tipados (`SetPRURL`, `GetPRURL`, `SetEpicBranch`, etc.).

### 3.4 Por que três camadas e não duas

Tentativa 1 (rebrand 1:1, só Camada 1): força o workflow a colapsar em 4-5 estados; perde fidelidade necessária pra distinguir "agent done, aguardando merge" de "épico aguardando humano aprovar PR".

Tentativa 2 (estado fino em `bd.metadata`): o bd não expõe metadata JSON arbitrária como campo de primeira classe; gastown não usa esse caminho — usa **description**. Manter a mesma escolha mantém compatibilidade conceitual e operacional com o ecossistema bd.

Tentativa 3 (store SQLite próprio do kernl): adianta a Fase 5 do MVP (run-state durável) só pra resolver workflow; cria duas fontes de verdade desnecessariamente cedo.

A arquitetura escolhida (status built-in + status custom + description fields) é exatamente o que o gastown rodou em produção e é o caminho com menor superfície de risco e maior alinhamento com o ecossistema bd.

---

## 4. Workflow do kernl (estados)

### 4.1 IssueStatus (Camadas 1 + 2)

| Status | Origem | Aplica em | Significado |
|---|---|---|---|
| `open` | bd built-in | filho e épico | Criado pela skill (`vc-convert-plan-to-beads` emite assim). `bd ready` filtra os com deps OK. |
| `in_progress` | bd built-in | filho e épico | Kernl claimou. Worker agent ativo na worktree do filho. Merger agent ativo no épico durante o passo de integração. |
| **`awaiting_integration`** | **kernl custom** | filho | Worker terminou OK. Branch da worktree-filho pronta pra ser mergeada pelo MergeManager quando todos os irmãos estiverem prontos. |
| **`awaiting_pr_review`** | **kernl custom** | épico | MergeManager concluiu: branch do épico mergeada com todos os filhos + PR aberta em direção à master. Aguarda humano aprovar/mergear. |
| `blocked` | bd built-in | filho e épico | Falha terminal — agent travou (watchdog), cap de follow-up esgotado, conflito de merge não-resolvido, `gh pr create` falhou. Épico para; humano corrige e re-roda `kernl epic run`. |
| `closed` | bd built-in | filho e épico | Filho: mergeado na epic branch. Épico: PR mergeado em master (fechado pelo `kernl sweep`, não manual). |

**Total de custom statuses:** 2 (`awaiting_integration`, `awaiting_pr_review`).

### 4.2 Tipos Go (em `orchestrator/internal/workflow/status.go`)

Espelhando `gastown/internal/beads/status.go`:

```go
package workflow

type IssueStatus string

const (
    StatusOpen                IssueStatus = "open"
    StatusInProgress          IssueStatus = "in_progress"
    StatusAwaitingIntegration IssueStatus = "awaiting_integration"  // custom
    StatusAwaitingPRReview    IssueStatus = "awaiting_pr_review"    // custom
    StatusBlocked             IssueStatus = "blocked"
    StatusClosed              IssueStatus = "closed"
)

func (s IssueStatus) IsTerminal() bool      // closed
func (s IssueStatus) IsClaimableByWorker() bool   // open
func (s IssueStatus) HaltsEpic() bool       // blocked
func (s IssueStatus) IsCustom() bool        // awaiting_integration, awaiting_pr_review

// Lista dos customs que o kernl registra via EnsureCustomStatuses.
var KernlCustomStatuses = []string{
    string(StatusAwaitingIntegration),
    string(StatusAwaitingPRReview),
}
```

### 4.3 Diagrama de transições

```
┌─── filho (bead-filho de um épico) ──────────────────────────────────┐
│                                                                      │
│  open ──claim──> in_progress ──agent_done──> awaiting_integration   │
│                       │                              │               │
│                       └──agent_failed/stuck──> blocked               │
│                                                      │               │
│                            merger pega ──> closed (mergeado na       │
│                                                     epic branch)     │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘

┌─── épico (parent bead) ─────────────────────────────────────────────┐
│                                                                      │
│  open ──claim──> in_progress ──todos filhos done──>                  │
│                       │             merger spawnado                  │
│                       │             merger termina OK                │
│                       │                  │                           │
│                       │                  └──> awaiting_pr_review     │
│                       │                              │               │
│                       │                              │ kernl sweep   │
│                       │                              │ detecta PR    │
│                       │                              │ mergeado      │
│                       │                              ▼               │
│                       └──merger falha──> blocked    closed           │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 4.4 AgentState — runtime do agent em JSON local

**[AMENDED 2026-05-15 — D1=C]** AgentState vive em `~/.kernl/state/<bead-id>.json` (NÃO em description). Schema:

```json
{
  "agent_state": "working",
  "agent_session_id": "<opencode session id>",
  "agent_started_at": "2026-05-15T14:23:00Z",
  "last_heartbeat_at": "2026-05-15T14:25:30Z",
  "follow_up_count": 2
}
```

| AgentState | Quando | Transição típica |
|---|---|---|
| `spawning` | Worktree criada, processo opencode iniciando. | → `working` (ack do agent) ou → `failed` (spawn falhou) |
| `working` | Agent ativo, take loop avançando. | → `done` (sucesso) / `stuck` (watchdog) / `failed` (erro terminal) |
| `done` | Agent reportou conclusão. Marcador momentâneo antes do executor transicionar o `IssueStatus` para `awaiting_integration` (filho) ou `awaiting_pr_review` (épico). Preservado como audit trail. | terminal |
| `stuck` | Watchdog detectou estagnação (sem progresso > N min). | Executor decide: retry/follow-up ou marca `IssueStatus=blocked` |
| `failed` | Agent retornou erro terminal ou cap de follow-up esgotado. | `IssueStatus` vira `blocked` |

**Reusado por worker E merger.** É o mesmo conceito — "agent qualquer fazendo trabalho contra esta bead". O prompt difere; o estado de runtime é o mesmo vocabulário.

**Garantias do `agent_state_store.go`** (D2, D9, D12):
- **bd é SoT pra trigger logic** (D2=A) — MergeManager/EpicExecutor SEMPRE consultam `bd list --status=...` pra decidir transição, NUNCA leem o JSON local pra isso. JSON é observability/audit.
- **Ordem canônica de write** — bd update primeiro, JSON depois. Crash entre os dois: bd vence; JSON pode reset com defaults sem perda funcional.
- **Atomic write** via tempfile + rename (D12=A).
- **Mutex em-processo** por bead_id pra serializar escritas concorrentes do mesmo processo.
- **Read sob arquivo ausente/corrompido** → defaults + log WARN (D9=A); usuário pode `rm -rf ~/.kernl/state/` pra reset sem brick.
- **Purge no `bd close`** — bead fechado → arquivo deletado (cleanup hook no EpicExecutor).

**Não incluído no MVP:** `awaiting-gate`, `escalated`, `paused`, `idle`, `patrolling`, `nuked` (conceitos do gastown que kernl não tem ainda). `schema_version` no JSON também fica fora — D9=A escolheu recover puro sem versionar; versionamento posterga.

### 4.5 Campos estáveis em description (Camada 3)

**[AMENDED 2026-05-15 — D1=C]** Apenas campos estáveis vivem em description. Padrão `key: value` por linha (`getMetadataField` / `addMetadataField`):

**Em filhos:**
```
worktree_path: /home/gabriel/.kernl/worktrees/kernl-abc/kernl-def
worktree_branch: feat/kernl-def
```

**Em épicos:**
```
epic_branch: feat/kernl-abc
pr_url: https://github.com/.../pull/42     (preenchido após gh pr create)
merge_conflict_at: feat/kernl-def          (se conflito não-resolvido)
merge_outcome: success                      (enum: success/merge_conflict/push_failed/pr_create_failed/pr_already_exists)
```

### 4.6 Combinações válidas `(IssueStatus × AgentState)`

**[NEW 2026-05-15 — TT5=A]** Outside voice flag: separação ortogonal não é simplificação real — espaço de estados efetivo é o produto cartesiano. Combinações inválidas devem ser detectáveis em runtime.

```
                       | spawning | working | done | stuck | failed |
-----------------------|----------|---------|------|-------|--------|
open                   |    N     |    N    |  N   |   N   |   N    |
in_progress            |    Y     |    Y    |  Y   |   Y   |   Y    |
awaiting_integration   |    N     |    N    | Y*   |   N   |   N    |
awaiting_pr_review     |    N     |    N    | Y*   |   N   |   N    |
blocked                |    N     |    N    |  N   |   Y   |   Y    |
closed                 |    N     |    N    | Y*   |   N   |   N    |
```

`Y*` = válido apenas como audit trail (estado final do agent preservado). `N` = inválido — runtime assertion deve falhar.

`workflow/status.go` expõe:
```go
func IsValidCombination(s IssueStatus, a AgentState) bool
```

Testes exaustivos via table-test (todas as combinações). Runtime assertions em writes detectam regressão.

### 4.7 Reasons em `bd close`

`closed` é estado terminal único. Spec §8.4 mapeia legacy `deferred` e `abandoned` (do foolery) pra `closed` com reason:

- `bd close <id> --reason="deferred"` — pausa intencional pelo humano (não é blocked, que tem semantic "kernl precisa de intervenção").
- `bd close <id> --reason="abandoned"` — decisão humana de não fazer.
- `bd close <id> --reason="aborted"` — abort de épico em andamento (manual no MVP via `bd close` por bead; subcomando `kernl epic abort` fica como TODO T3 pós-MVP — ver §10).

Runtime do kernl **nunca gera essas reasons autonomamente** — só aparecem quando humano roda `bd close --reason=...`.

---

## 5. MergeManager + agente de integração automática

### 5.1 Decisão de escopo

O brainstorm-spec do MVP anterior (D4) previa **review/merge manual no fim do épico** e o "loop agêntico completo de integração" como bloco seguinte. **Esta spec antecipa uma fatia mínima desse bloco pro MVP** — porque o "manual" significava o usuário abrir uma sessão interativa e dizer "mergeia as worktrees, resolva conflitos, abra PR", o que é equivalente a o orquestrador disparar essa sessão automaticamente quando a condição é satisfeita.

Esta antecipação muda o **D4 do spec anterior**: o loop de integração não fica inteiro fora do MVP — entra a versão mínima (1 agente, 1 prompt, watchdog reusado).

### 5.2 Lifecycle do épico no novo modelo

1. **Início do `kernl epic run <epic-id>`**: `WorktreeManager` cria a **epic branch** `feat/<epic-id>` a partir de `master` **[D8=A: id puro, determinístico — PR title carrega o nome legível via `gh pr create --title <epic.title>`]**.
2. **Worktrees dos filhos**: cada filho `git worktree add` partindo da epic branch (não de master), em sua própria branch `feat/<child-bead-id>`.
3. **Workers rodam em paralelo** (respeitando o DAG de deps). Cada filho `awaiting_integration` quando seu worker reporta `done`.
4. **Trigger do merger**: `EpicExecutor` detecta "todos os filhos do épico em `awaiting_integration`" via **single `bd list --parent=<epic-id> --status=awaiting_integration --json`** [D14=A] e compara contagem com total de filhos. Detecção protegida por **single-flight lock por `epic_id`** [D11=A] pra evitar duplo trigger. Marca o épico-bead `in_progress`, dispara o merger agent contra o épico worktree.
5. **Merger agent executa** (ver §5.3) — merge sequencial em ordem topológica, push, abre PR. **Validação empírica (TT1):** spike em 2026-05-15 rodou contra foolery-go (3 epics simuladas, 9 merges válidos) — taxa de conflito 0%. Design batch-no-fim validado.
6. **Outcome reportado pelo agent** (ver §5.3, §5.4) — agent escreve `merge_outcome: <enum>` na description do épico antes de terminar. MergeManager parseia e roteia transições determinísticamente.
7. **Sucesso (`merge_outcome: success` ou `pr_already_exists`)**: filhos viram `closed`, épico vira `awaiting_pr_review` (com `pr_url:` preenchido na description).
8. **Humano aprova/mergeia o PR no GitHub** (gate humano).
9. **`kernl sweep`** detecta PR mergeado e fecha o épico-bead automaticamente (ver §6).

### 5.3 Merger agent prompt (em `orchestrator/internal/prompt/merger_prompt.go`)

**[AMENDED 2026-05-15 — D3=A, D6=A]** Template renderizado com:
- `epic_id`, `epic_title`, `epic_branch` (sempre `feat/<epic-id>` per D8=A)
- Lista ordenada (topologicamente) de `(child_bead_id, child_branch, child_worktree_path)`
- `base_branch` (master)
- Lista literal dos outcomes válidos do enum `merge.Outcome` (ver `merge/errors.go`)

**Instruções concretas no prompt:**
1. `cd <epic worktree>`, `git checkout <epic_branch>`.
2. Loop de `git merge --no-ff <child_branch>` em ordem topológica.
3. **Em conflito:** ler markers, decidir, editar, `git add`, `git commit`. Se não convergir em N tentativas (cap de follow-up) — escrever `merge_outcome: merge_conflict` + `merge_conflict_at: <branch>` na description e terminar.
4. Após todos merges OK, `git push origin <epic_branch>` — **retry 2× com backoff** em falha; se ainda falhar → escrever `merge_outcome: push_failed` e terminar.
5. `gh pr create --title <epic.title> --body <body auto-gerado>`. **Se erro indica PR já existe** (re-run idempotente) → buscar URL existente via `gh pr list --head <epic_branch> --json url`, escrever `pr_url: ...` + `merge_outcome: pr_already_exists`, terminar success-equivalent. **Demais erros** → escrever `merge_outcome: pr_create_failed` + terminar.
6. **Sucesso completo:** escrever `pr_url: <url>` + `merge_outcome: success` na description do épico, terminar.

**Outcomes que o prompt PODE escrever** (lista literal — também no enum `merge.Outcome` em `merge/errors.go`):
- `success`
- `merge_conflict`
- `push_failed`
- `pr_create_failed`
- `pr_already_exists`

Custo: ~130 LOC de prompt + ~30 LOC de render em Go + ~50 LOC de enum/parser em `merge/errors.go`.

**TODO follow-up (não no MVP):** estratégia de contexto pro merger — que arquivos do repo o merger lê pra ser inteligente em conflitos. Candidatos: `AGENTS.md`, últimos N commits da master, descrição do épico-bead, descrições dos filhos. Documentado como design item pra próxima iteração da spec do prompt.

### 5.4 Política de conflito de merge + transições do MergeManager

**[AMENDED 2026-05-15 — D3=A]** MergeManager (Go) lê `merge_outcome:` da description do épico após o agent terminar (`agent_state: done` no JSON local) e roteia transições determinísticamente:

```
merge_outcome           | IssueStatus transition          | Side effects
------------------------|---------------------------------|--------------------------------------
success                 | épico → awaiting_pr_review     | filhos → closed
pr_already_exists       | épico → awaiting_pr_review     | filhos → closed (PR existente adotado)
merge_conflict          | épico → blocked                 | merge_conflict_at preserved; sem touch nos filhos
push_failed             | épico → blocked                 | nem todos filhos closed; humano avalia
pr_create_failed        | épico → blocked                 | branch pushada, mas sem PR; humano avalia
(ausente ou inválido)   | épico → blocked                 | log ERROR "merger agent did not report outcome"
```

**Política de conflito MVP:** o merger agent tenta resolver conflitos. Se não convergir em N tentativas (cap de follow-up) ou watchdog detecta estagnação → escreve `merge_outcome: merge_conflict` + `merge_conflict_at: <branch>` → MergeManager marca épico `blocked`.

**Recovery manual:** humano resolve no git diretamente (no épico worktree, `git merge --continue` / `git mergetool` / etc.) e roda `kernl epic merge <epic-id>` [D4=A — ver §5.7] pra re-disparar o passo de integração. O subcomando valida:
- Épico em `blocked` com `merge_conflict_at` setado.
- Filhos em `awaiting_integration` ou `closed`.
- Re-marca épico `in_progress`, dispara MergeManager.
- Idempotente — re-rodar em estado errado retorna erro descritivo.

**Fora do MVP (SotA — agente resolvedor especializado):** sub-bead dinâmico de "resolution_agent" com prompt especializado em git conflict resolution, validações estritas (testes têm que passar), múltiplas estratégias de resolução. Registrado na §10 como item futuro.

### 5.5 Testes pós-merge antes do PR

**Decisão MVP: NÃO rodar.** A CI do PR cobre isso quando humano aprova. Manter o merger focado em "merge + push + PR" reduz superfície de erro. Configurável depois (`kernl.yaml: epic_post_merge_command`).

### 5.6 `gh pr create` no fim

Disparado pelo merger agent dentro do prompt. Body auto-gerado: título do épico + lista de filhos mergeados (com IDs e títulos) + link pra spec se houver na description do épico.

### 5.7 Componentes novos

**[AMENDED 2026-05-15 — D4=A, D6=A, D11=A]**

| Pacote | Responsabilidade | LOC estimado |
|---|---|---|
| `orchestrator/internal/merge/manager.go` | `MergeManager`: detect trigger via single `bd list` query (D14), single-flight lock por epic_id (D11), dispatch merger agent, parse `merge_outcome` (D6) e roteia transitions (D3). | ~180 |
| `orchestrator/internal/merge/errors.go` (NOVO) | Enum tipado `Outcome` + `ParseOutcome` + helpers de transition routing (D6=A). | ~50 |
| `orchestrator/internal/prompt/merger_prompt.go` | Template + render. Enumera 3 failure modes + lista literal dos outcomes (D3=A). | ~150 |
| `orchestrator/internal/workflow/agent_state_store.go` (NOVO) | JSON local store em `~/.kernl/state/<bead-id>.json` (D1=C). Atomic write tempfile+rename (D12=A). Mutex em-processo. Recover-on-corrupt (D9=A). Purge no `bd close`. | ~120 |
| `orchestrator/cmd/kernl/epic_merge.go` (NOVO) | Subcomando `kernl epic merge <epic-id>` — re-dispatch merger após conflito resolvido manualmente (D4=A). Validações de pre-flight. | ~60 |
| Adição em `orchestrator/internal/worktree/` | Criar epic branch `feat/<epic-id>` (D8=A) antes das worktrees dos filhos. | ~50 |
| Tests | Unit (incluindo single-flight determinístico, outcome parsing exaustivo) + integration (3 E2E paths críticos — ver test plan). | ~280 |
| **Total incremental** | | **~890** |

### 5.8 Reuso

| Componente | Reuso | Origem |
|---|---|---|
| Take loop / watchdog / follow-up cap | 100% reuso. | `terminal/` do foolery-go portado |
| Dispatch de agent | 100% reuso. | `dispatch/` do foolery-go portado |
| AgentState (lifecycle de qualquer agent) | 100% reuso. | Definido nesta spec (§4.4) |
| SSE / observabilidade | 100% reuso. | `SessionConnectionManager` já portado |

Nenhuma infra nova é necessária pro merger além do MergeManager + prompt — toda a maquinaria de "spawnar agent, monitorar, detectar travamento, encerrar" já existe.

---

## 6. `kernl sweep` — fechamento automático de beads

### 6.1 Problema

Após o humano aprovar e mergear o PR na master, os beads do épico (filhos + parent) seguem em status `awaiting_pr_review`/`closed`. **Fechar manualmente via `bd close <id>` é péssima DX** — especialmente sem UI dedicada.

### 6.2 Lógica do `kernl sweep`

**[AMENDED 2026-05-15 — D5=C, TT2=B]** Resiliência via 3 camadas:

```
para cada épico em bd list --status=awaiting_pr_review:
    pr_url = getMetadataField(epic.description, "pr_url")
    se pr_url vazio:
        skip (anomalia — logar WARN)
        continue

    # Camada 1 — Cache MERGED (D5=C)
    se cache[pr_url] == MERGED:
        # Fecha idempotente (caso bd close anterior tenha falhado)
        bd close (silencioso se já closed)
        continue

    # Camada 2 — Circuit breaker (D5=C)
    se breaker.open(épico.id):
        skip (em backoff exponencial 5/15/60min)
        continue

    # Camada 3 — gh call com error handling
    pr_state = gh pr view <pr_url> --json state,mergedAt,mergeCommit,createdAt
    em erro:
        breaker.fail(épico.id)
        log WARN (epic_id, err)
        continue

    breaker.success(épico.id)   # reset counter

    # PR stale WARN (TT2=B)
    days_open = now - pr_state.createdAt
    se pr_state.state == "OPEN" e days_open > config.pr_stale_warn_days:
        log WARN "PR <url> aberto há <days_open> dias"

    se pr_state.state == "MERGED":
        cache[pr_url] = MERGED
        para cada filho em bd list --parent=<epic-id>:
            se filho.status != closed:
                bd close <filho-id> --reason="merged via PR <url> at <mergedAt>"
        bd close <epic-id> --reason="merged via PR <url> at <mergedAt>"
```

GitHub é fonte da verdade — `mergedAt: not null` = "código está em master". `gh` CLI já entrega isso em JSON pronto.

### 6.3 Modos de operação

| Modo | Trigger | Uso |
|---|---|---|
| **Subcomando `kernl sweep`** | Manual: usuário roda no terminal. | Hábito casual após fechar PR. Equivalente moral de `bd sync`. |
| **Auto-tick em `kernl serve`** | Background: o server faz sweep periodicamente. | Quando GUI está aberta, beads fecham sozinhos enquanto usuário trabalha em outra coisa. |
| **`kernl sweep --dry-run`** | Manual com `--dry-run`. | Preview do que seria fechado, sem efetuar. |

Config no `kernl.yaml`:

```yaml
sweep:
  auto_interval_seconds: 60      # 0 = desabilitado
  github_token_env: GH_TOKEN     # se precisar de auth não-default
  pr_stale_warn_days: 7          # TT2=B — warn quando PR aberto há > N dias (0 = desabilitado)
  circuit_breaker:               # D5=C
    failure_threshold: 3         # falhas consecutivas antes de abrir
    backoff_minutes: [5, 15, 60] # progressão de backoff
```

### 6.4 Output

```
$ kernl sweep --dry-run
Found 2 epics with merged PRs:
  kernl-abc (PR #42) — would close 5 child beads + epic
    children: kernl-def, kernl-ghi, kernl-jkl, kernl-mno, kernl-pqr
  kernl-xyz (PR #47) — would close 3 child beads + epic
    children: kernl-rst, kernl-uvw, kernl-zab

Run without --dry-run to apply.
```

### 6.5 Componente

`orchestrator/internal/sweep/` — totalmente isolado do MergeManager e do EpicExecutor. Não tem acoplamento com a state machine além de **ler** `bd list --status=awaiting_pr_review` e **escrever** via `bd close`.

LOC estimado: ~200 (core + dry-run + subcomando + auto-tick) + ~150 (testes).

---

## 7. Decisões

| ID | Decisão | Razão |
|----|---------|-------|
| W1 | Arquitetura de persistência em 3 camadas (built-in + custom + description), espelhando o gastown. | Resolveu exatamente o mesmo problema em produção; cargo cult deliberado. |
| W2 | 6 estados de `IssueStatus`: 4 built-in (`open`, `in_progress`, `blocked`, `closed`) + 2 custom (`awaiting_integration`, `awaiting_pr_review`). | Cobre o ciclo de vida real do kernl sem inventar estados de planning/shipment que não existem mais no MVP. |
| W3 | `AgentState` separado de `IssueStatus`, ortogonal, em `description` como `agent_state: <valor>`. | Conceitos diferentes; gastown separou e funciona. |
| W4 | Reuso do `AgentState` entre worker e merger (mesmo vocabulário pra qualquer agent). | "Que estado um agent está em" é um conceito único; o prompt difere, o estado de runtime não. |
| W5 | MergeManager + agente de integração automática **entram no MVP**. | "Manual" equivalia a o usuário disparar a mesma sessão interativa manualmente. Automatizar custa ~530 LOC totais e elimina toda fricção. Sobrescreve D4 do spec MVP anterior. |
| W6 | Política de conflito MVP: agent **tenta resolver**, watchdog cobre escapes, bead vira `blocked` se não converge. | Coerente com a tese "humano só toca em pontos de julgamento" — conflito intratável é ponto de julgamento. |
| W7 | `kernl sweep` como subcomando + auto-tick em `kernl serve` + `--dry-run`. | Fecha o loop "humano mergeia PR no GitHub → beads se atualizam sozinhos". DX coerente com o resto. |
| W8 | ~~Delete completo do knots~~ → **REVERTIDO pós-eng-review-2026-05-15 (D0=C):** knots delete vira follow-up PR. Workflow + MergeManager + sweep ficam no PR atual. | Big-bang com 3 movimentos independentes acopla riscos; knots delete sem payoff funcional pro MVP; contradiz decisão anterior da eng review de manter knots dormante no MVP. Mantém TODO "Remoção completa do knots" em `TODOS.md`. |
| W9 | Perfis customizáveis (`autopilot`/`semiauto`/etc.) deletados. | YAGNI no MVP. Voltam se aparecer caso de uso. |
| W10 | Migração big-bang em PR único (workflow + MergeManager + sweep, sem knots delete por D0=C). | kernl pré-MVP, sem produção a proteger; phased não ganha nada nos 3 movimentos do escopo. |
| W11 | Spec única cobrindo workflow + auto-merge + sweep + migração. | Acoplamento conceitual alto demais pra separar; ler 3 docs pra entender 1 coisa é pior. |
| W12 | **[NEW eng review 2026-05-15 — D1=C]** AgentState (runtime do agent) **sai do bd.description** e vai pra JSON local em `~/.kernl/state/<bead-id>.json`. Description guarda só campos estáveis. | Race de escritor concorrente em `bd.description` (worker heartbeat + merger update) causaria lost-update → watchdog falso positivo. Gastown evita via single-mux-server; kernl é multiprocess, então diverge conscientemente. Trade-off aceito: única divergência da arquitetura gastown. |
| W13 | **[NEW eng review 2026-05-15 — D2=A]** bd é **única SoT pra trigger logic** (MergeManager/EpicExecutor SEMPRE consultam `bd list`, NUNCA o JSON local pra decisões de transição). | JSON local guarda só runtime/observability — perdível em crash sem dano funcional. Elimina split-brain entre dois stores. |
| W14 | **[NEW eng review 2026-05-15 — D6=A + D3=A]** `merge.Outcome` é enum tipado em Go; merger agent escreve `merge_outcome: <enum>` na description do épico; MergeManager parseia via switch exaustivo e roteia transições determinísticamente. Failure modes (push_failed, pr_create_failed, pr_already_exists) enumerados explicitamente no prompt. | Type-safe, refactor-safe, compilador pega outcome novo não-tratado. Branch pushada sem PR não fica mais órfã (idempotência via pr_already_exists). |

---

## 8. Plano de migração (big-bang em PR único)

### 8.1 Pré-condição

- Rename foolery→kernl já consumado em `orchestrator/internal/` e `orchestrator/cmd/` (confirmado nesta sessão).
- Spec aprovada (este documento).

### 8.2 Mudanças concretas no PR `workflow/kernl-spec-migration`

**[AMENDED 2026-05-15 — D0=C: knots delete diferido pra follow-up PR]**

**Deletes (knots delete DIFERIDO — agora APENAS limpeza de state_machine e perfis):**
- ~~`orchestrator/internal/backend/knots.go`, `knots_test.go`~~ — **DIFERIDO pro follow-up PR (D0=C).** Knots permanece dormante neste PR.
- `orchestrator/internal/backend/state_machine.go`: deleta `profileConfig`, `builtinProfiles`, `agentOwners`, `semiautoOwners`, `normalizeProfileID`, `descriptorFromProfileConfig`, `initBuiltinWorkflows`, `BuiltinProfileDescriptor`, `resolveWorkflow`, `canonicalTransitions`, `buildStates`, `filterTransitions`, `deriveWorkflowStructureFromConfig`, `stepOwnerKind`. Sobra: ~150 LOC com o novo modelo simples.
- `orchestrator/internal/backend/port.go`: encolhe `WorkflowDescriptor` (tira `Owners`, `QueueActions`, `ActionStates`, `ReviewQueueStates`, `HumanQueueStates`, `StateOwners`, `FinalCutState`, `Mode`, etc.).
- `orchestrator/internal/backend/factory.go`: simplifica `bd` como caminho default; **knots permanece registrado mas nunca roteado** (decisão da eng review anterior preservada).
- Constantes `agent_owners`/`semiauto_owners`/`autopilot*` e perfis correlatos.

**Novos:**
- `orchestrator/internal/workflow/status.go` — `IssueStatus`, `AgentState`, `KernlCustomStatuses`, `IsValidCombination` (TT5=A), métodos semânticos.
- `orchestrator/internal/workflow/description.go` — `getMetadataField`, `addMetadataField`, helpers tipados pra campos **estáveis** apenas (D1=C: AgentState saiu daqui).
- `orchestrator/internal/workflow/agent_state_store.go` **(NOVO — D1=C, D9=A, D12=A)** — JSON local store em `~/.kernl/state/<bead-id>.json`. Atomic write tempfile+rename. Mutex em-processo. Recover-on-corrupt com defaults + WARN.
- `orchestrator/internal/workflow/ensure_custom.go` — `EnsureCustomStatuses(beadsDir)` idempotente com cache em memória + sentinel (cargo cult gastown, D7=B).
- `orchestrator/internal/merge/manager.go` — MergeManager: detect trigger via single bd list (D14), single-flight lock (D11), parse `merge_outcome` (D6), roteia transitions (D3).
- `orchestrator/internal/merge/errors.go` **(NOVO — D6=A)** — Enum tipado `Outcome` + `ParseOutcome` + helpers de routing.
- `orchestrator/internal/prompt/merger_prompt.go` — template do prompt enumerando 3 failure modes + lista literal dos outcomes (D3=A).
- `orchestrator/internal/sweep/sweep.go` — lógica do `kernl sweep` com cache MERGED + circuit breaker + PR stale WARN (D5=C, TT2=B).
- `orchestrator/cmd/kernl/sweep.go` — subcomando.
- `orchestrator/cmd/kernl/epic_merge.go` **(NOVO — D4=A)** — Subcomando `kernl epic merge <epic-id>` pra re-dispatch após conflito resolvido manualmente.
- Wiring no `kernl serve` pro auto-tick (config + goroutine).

**Refatorados:**
- `orchestrator/internal/backend/bdcli.go`: `Update` deixa de aceitar strings legacy; `Init`/`NewBdCliBackend` chama `EnsureCustomStatuses`; helpers de description-field são invocados no caminho de write quando o caller passa `AgentState` etc.
- `orchestrator/internal/dispatch/`, `orchestration/`, `terminal/`, `retake/`, `epic/`, `app/` — onde tiver string literal legacy (`"ready_for_implementation"`, etc.), troca pelas constantes novas do pacote `workflow`. Os testes (`*_test.go`) idem.
- `cmd/kernl/bead_test.go` — idem.
- `orchestrator/internal/integration/harness.go`, `passo_a_test.go` — idem (e fixtures, abaixo).

**Fixtures:**
- `orchestrator/internal/integration/testdata/beads-*/.beads/issues.jsonl` — `sed` global: troca de cada `"status":"<legacy>"` pelo equivalente novo. Mapping:
  - `ready_for_implementation` → `open`
  - `implementation` → `in_progress` (e setar `agent_state: working` na description, se o teste depender)
  - `implementation_review` → `awaiting_integration`
  - `ready_for_shipment` → `awaiting_integration`
  - `shipment` / `shipment_review` / `shipped` → `closed`
  - `deferred` → `closed` com `reason: "deferred"` (pausa intencional — preserva semantic legacy; **nunca** vira `blocked`, que tem significado diferente: "kernl precisa de intervenção pra prosseguir")
  - `abandoned` → `closed` com `reason: "abandoned"` (decisão de não fazer)
  - Se algum teste especificamente assert sobre a distinção foolery entre `deferred` e `closed`, o teste é **deletado** — o conceito sai do modelo. Runtime do kernl não gera `deferred` nem `abandoned` autonomamente; ambos só aparecem como reasons quando o **humano** roda `bd close <id> --reason=...` manualmente.

**Specs:**
- `orchestrator/specs/00-architecture.md` — limpa referência a `foolery.go` stale (linha 522), atualiza diagrama do workflow, remove menções a perfis.
- `orchestrator/specs/backend/backend.md` — atualizado pra refletir o novo write path (status built-in/custom + description), preservando provenance `[source: foolery/src/...]` apenas onde o behavior contract herdado segue valendo. Adiciona seção "Custom statuses" e "Description-field contracts".
- `orchestrator/specs/orchestration/orchestration.md` — atualiza o lifecycle do épico (com merger + sweep), o diagrama de transições.
- `orchestrator/specs/prompt/prompt.md` — adiciona seção do merger prompt + TODO do contexto-aware.
- (Outros specs tocados conforme inspeção.)

**Docs:**
- `TODOS.md` — fecha "Definir workflow próprio do kernl" + "Remoção completa do knots do codebase".

### 8.3 Gates de qualidade

**[AMENDED 2026-05-15 — D10=A pinagem bd ≥ 1.0.4]**

1. **962 unit tests passam** (após adaptação das assertions sobre status names).
2. **Passo A do MVP** (`kernl epic run <single-bead>` contra bd CLI real) passa pelo menos até o ponto que dependia desta migração.
3. **`bd doctor`** (do bd) não reporta status inválidos no `.beads/` do próprio kernl.
4. **`go vet ./...` + linters** limpos.
5. **Pinagem bd ≥ 1.0.4 (D10=A)** — `internal/integration/harness.go` valida `bd --version` no SetUp com fail-fast; CI instala versão pinada via `go install bd@v1.0.4`. Documentado em `AGENTS.md` e README.
6. **3 E2E paths críticos** (ver `docs/reviews/vc-plan-eng-review-test-plan-2026-05-15.md`): happy-path, conflict+recovery via `kernl epic merge`, push-fail+recovery — todos passam contra bd 1.0.4 real.

### 8.4 Risco residual

- **Description-field parsing edge cases:** linhas com `:` no valor, escapes, BOM, etc. Mitigação: copiar a regex/string handling do gastown 1:1 — eles já bateram nesses casos.
- **EnsureCustomStatuses race condition:** dois processos kernl tentando registrar simultaneamente. Mitigação: gastown usa `mkdir` atômico + sentinel file na pasta `.beads/`; copiar o padrão (D7=B).
- **Single-flight lock no MergeManager:** trigger detection de "todos filhos done" pode em teoria disparar dois mergers se o lock falha. Mitigação: D11=A — `sync.Map[epic_id]chan struct{}` + test determinístico com N goroutines simultâneas (sync.WaitGroup, sem timing aleatório).
- **JSON corrompido no `~/.kernl/state/`:** crash entre write tempfile e rename, ou edit manual do usuário. Mitigação: D9=A — recover com defaults + WARN; usuário pode `rm -rf ~/.kernl/state/` pra reset sem brick.
- **Knots dormante:** mantém superfície de código não-utilizada (14 arquivos). Mitigação: TODO existente "Remoção completa do knots" cobre o follow-up PR.
- **Fixtures com estado legacy que era semanticamente único** (`implementation_review` vs `shipment_review`): podem precisar de remodelagem por teste. Mitigação: revisar cada fixture caso a caso na PR review; alguns testes podem ser repensados ou removidos.
- **Conflict rate em produção:** spike empírico TT1 (3 epics simuladas de foolery-go, 9 merges válidos) mostrou 0% de conflito. Premissa validada. Se taxa crescer em uso real, pivotar pra incremental-merge per filho (registrado como design knob futuro).

---

## 9. Critérios de sucesso (verificáveis)

**[AMENDED 2026-05-15 — D0=C alterou critério #7]**

1. `kernl epic run <epic-id>` contra bd 1.0.4 real avança o estado do bead sem `validation failed`.
2. Após todos os filhos terminarem, o merger agent é despachado automaticamente e merge das worktrees-filhas → epic branch acontece.
3. Em caminho-feliz (sem conflito), o `gh pr create` roda e o épico fica em `awaiting_pr_review` com `pr_url:` na description.
4. `kernl sweep --dry-run` lista épicos com PR mergeado sem efetuar.
5. `kernl sweep` (sem `--dry-run`) fecha filhos e épico após PR aprovado/mergeado em master.
6. `bd ready` no kernl não retorna nenhum bead com status legacy do foolery (smoke-check: a refatoração foi completa).
7. ~~Zero referências a knots em `orchestrator/internal/` e `orchestrator/cmd/`.~~ **[REVISTO D0=C]** Knots permanece dormante neste PR (nunca roteado pelo factory). Critério "zero referências" move-se pro follow-up PR de knots delete.
8. Conflito de merge intratável faz o épico ir pra `blocked`, e `kernl epic merge <epic-id>` re-dispara o passo após resolução manual.
9. **[NEW D6=A]** `merge_outcome:` na description do épico após merger completar; MergeManager parseia via enum tipado e roteia transitions corretamente (success/merge_conflict/push_failed/pr_create_failed/pr_already_exists).
10. **[NEW D1=C]** `~/.kernl/state/<bead-id>.json` criado/atualizado durante o lifecycle do agent; recover-on-corrupt sem brick; arquivo purged no `bd close`.
11. **[NEW D5=C]** Sweep com circuit breaker e cache MERGED: 3 falhas consecutivas → backoff 5/15/60min; PR MERGED nunca re-consultado.

---

## 10. Fora de escopo (backlog explícito)

**[AMENDED 2026-05-15 — eng review adicionou T1, T2, T3]**

- **Remoção completa do knots** — TODO existente em `TODOS.md`. **D0=C diferiu pra follow-up PR** (era W8 original desta spec).
- **Property-based race test pro trigger "todos filhos awaiting_integration"** — **NOVO TODO T1** em `TODOS.md` (D11B). Complementa o test determinístico do D11=A.
- **Batch heartbeats em memória no AgentStateStore** — **NOVO TODO T2** em `TODOS.md` (D12B). Otimização pra ambientes IOPS-restritos; instrumentar só após evidência real.
- **Subcomando `kernl epic abort`** — **NOVO TODO T3** em `TODOS.md` (TT4=B). Caminho limpo pra cancelar épico em andamento; no MVP é manual via `bd close`.
- **PR staleness TTL com ação automática** — TT2=B no MVP é só WARN. Ação automática (auto-blocked após N dias) fica eventual TODO se padrão de uso justificar.
- **`schema_version` no JSON do agent_state** — D9=A escolheu recover puro; versionamento posterga.
- **Agente resolvedor de conflito especializado** (Self-Healing Merges do SotA) — sub-bead dinâmico com prompt especializado, múltiplas estratégias, validação rigorosa.
- **AST-aware paralelism detection** — detectar dependências lógicas entre beads (não só file-overlap) antes de despachar.
- **GitHub webhook pra sweep** — substituir polling por webhook quando o kernl evoluir pra modo multi-user ou tiver server público.
- **Perfis customizáveis** (`autopilot`/`semiauto`/etc.) — voltam se aparecer caso de uso real, com design renovado.
- **Estados de gate humano por bead** (`awaiting-gate` do gastown) — entram quando o conceito de gates discretos por bead aparecer.
- **AgentState extra** (`escalated`, `paused`, `idle`, `patrolling`, `nuked`) — entram conforme features que os justifiquem.
- **Testes pós-merge antes do PR** — configuráveis depois (`epic_post_merge_command`).
- **Limpeza automática de worktrees** (TODO existente em `TODOS.md`) — independente desta spec.
- **Estratégia de contexto pro merger prompt** — quais arquivos o merger lê pra ser inteligente em conflitos. Decisão pra próxima iteração da spec do prompt.

---

## 11. Próximos passos

**[AMENDED 2026-05-15 — spec passou por eng review, spike empírico TT1 executado]**

1. ✅ Spec commitada em `docs/` e amended pós-eng-review.
2. ✅ Eng review rodada — 14 decisões + 5 cross-model tensions resolvidas (ver `docs/reviews/vc-plan-eng-review-2026-05-15.md`).
3. ✅ Spike empírico TT1 executado contra foolery-go — 0% taxa de conflito em 9 merges válidos; design batch validado.
4. **Próximo:** `vc-writing-plans` produz o plano de implementação com bead mapping metadata.
5. Plano de implementação → `vc-convert-plan-to-beads` → execução pelo próprio kernl (dogfooding na primeira oportunidade real após o MVP rodar).

---

## 12. Apêndice — Referências de código no gastown

| Conceito | Arquivo |
|---|---|
| IssueStatus + AgentState com tipos e métodos | `internal/beads/status.go` |
| `EnsureCustomStatuses` + `EnsureCustomTypes` (idempotente, cache + sentinel) | `internal/beads/beads_types.go:187` |
| Lista de custom statuses registrados | `internal/constants/constants.go:202` |
| `getMetadataField` / `addMetadataField` na description | `internal/beads/integration.go:69-128` |
| `ParseAgentFields` (parser de campos estruturados em description) | `internal/beads/fields.go` (procurar `ParseAgentFields`) |
| Estado global do gastown enable/disable (não copiar — é outro escopo) | `internal/state/state.go` |

Toda referência deve ser tratada como **modelo de design**, não cópia literal — os domínios diferem (gastown gerencia agent swarms via tmux + mux servers; kernl orquestra workers em worktrees via opencode). Os mecanismos de **persistência e taxonomia de estados** são o que se reusa.
