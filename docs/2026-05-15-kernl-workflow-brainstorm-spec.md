# Brainstorm Spec — Workflow do Kernl

**Data:** 2026-05-15
**Escopo:** Definição do workflow próprio do kernl (substitui a state machine herdada do foolery), arquitetura de persistência espelhando o gastown, MergeManager + agente de integração automática, subcomando `kernl sweep`, e plano de migração big-bang.
**Status:** design aprovado — pronto para `vc-plan`.
**Referências cruzadas:**
- TODO "Definir workflow próprio do kernl (substituir state machine herdada do foolery)" em `TODOS.md`
- Brainstorm-spec do MVP do núcleo: `docs/2026-05-14-orchestration-nucleo-mvp-brainstorm-spec.md`
- Eng review: `docs/reviews/vc-plan-eng-review-2026-05-14.md`
- Strategy: `docs/STRATEGY.md`
- Gastown como referência arquitetural: `/home/gabriel/repositories/_cloned/gastown/internal/beads/status.go`, `internal/beads/integration.go`, `internal/beads/beads_types.go`, `internal/constants/constants.go`

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

Estado de runtime granular (o que o agente está fazendo agora, caminhos de worktree, branches, IDs de sessão, contadores, timestamps) **não vai no `status` nem em campo separado** — vai como linhas `key: value` dentro da `description` do bead.

Padrão de parsing/writing espelhando `gastown/internal/beads/integration.go:69-128`:

```go
// getMetadataField extrai "key: value" da description (case-insensitive na key).
// addMetadataField insere/atualiza idempotentemente.
```

Tipos Go separados pra cada conceito embutido. Pra runtime do agent: `AgentState` (`gastown/internal/beads/status.go:12-29` é o modelo).

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

### 4.4 AgentState (Camada 3) — runtime do agent em description

Vive como `agent_state: <valor>` na `description` do bead. Ortogonal ao `IssueStatus`. Só o kernl lê/escreve.

| AgentState | Quando | Transição típica |
|---|---|---|
| `spawning` | Worktree criada, processo opencode iniciando. | → `working` (ack do agent) ou → `failed` (spawn falhou) |
| `working` | Agent ativo, take loop avançando. | → `done` (sucesso) / `stuck` (watchdog) / `failed` (erro terminal) |
| `done` | Agent reportou conclusão. Marcador momentâneo antes do executor transicionar o `IssueStatus` para `awaiting_integration` (filho) ou `awaiting_pr_review` (épico). Preservado como audit trail. | terminal |
| `stuck` | Watchdog detectou estagnação (sem progresso > N min). | Executor decide: retry/follow-up ou marca `IssueStatus=blocked` |
| `failed` | Agent retornou erro terminal ou cap de follow-up esgotado. | `IssueStatus` vira `blocked` |

**Reusado por worker E merger.** É o mesmo conceito — "agent qualquer fazendo trabalho contra esta bead". O prompt difere; o estado de runtime é o mesmo vocabulário.

**Não incluído no MVP:** `awaiting-gate`, `escalated`, `paused`, `idle`, `patrolling`, `nuked` (conceitos do gastown que kernl não tem ainda — sem agent-level gates, sem deacon, sem polecat manager). Voltam se o caso de uso aparecer.

### 4.5 Campos adicionais em description

Mesmo padrão de `key: value` por linha (`getMetadataField` / `addMetadataField`). Cada um lido/escrito por um helper tipado:

**Em filhos:**
```
agent_state: working
agent_session_id: <opencode session id>
agent_started_at: 2026-05-15T14:23:00Z
last_heartbeat_at: 2026-05-15T14:25:30Z
follow_up_count: 2
worktree_path: /home/gabriel/.kernl/worktrees/kernl-abc/kernl-def
worktree_branch: feat/kernl-def
```

**Em épicos:**
```
agent_state: working                       (durante o merger)
epic_branch: feat/kernl-abc
pr_url: https://github.com/.../pull/42     (preenchido após gh pr create)
merge_conflict_at: feat/kernl-def          (se conflito não-resolvido)
```

---

## 5. MergeManager + agente de integração automática

### 5.1 Decisão de escopo

O brainstorm-spec do MVP anterior (D4) previa **review/merge manual no fim do épico** e o "loop agêntico completo de integração" como bloco seguinte. **Esta spec antecipa uma fatia mínima desse bloco pro MVP** — porque o "manual" significava o usuário abrir uma sessão interativa e dizer "mergeia as worktrees, resolva conflitos, abra PR", o que é equivalente a o orquestrador disparar essa sessão automaticamente quando a condição é satisfeita.

Esta antecipação muda o **D4 do spec anterior**: o loop de integração não fica inteiro fora do MVP — entra a versão mínima (1 agente, 1 prompt, watchdog reusado).

### 5.2 Lifecycle do épico no novo modelo

1. **Início do `kernl epic run <epic-id>`**: `WorktreeManager` cria a **epic branch** `feat/<epic-title-or-id>` a partir de `master`.
2. **Worktrees dos filhos**: cada filho `git worktree add` partindo da epic branch (não de master), em sua própria branch `feat/<child-bead-id>`.
3. **Workers rodam em paralelo** (respeitando o DAG de deps). Cada filho `awaiting_integration` quando seu worker reporta `done`.
4. **Trigger do merger**: `EpicExecutor` detecta "todos os filhos do épico em `awaiting_integration`" → cria/marca o épico-bead `in_progress`, dispara o merger agent contra o épico worktree.
5. **Merger agent executa** (ver §5.3) — merge sequencial em ordem topológica, push, abre PR.
6. **Sucesso**: filhos viram `closed`, épico vira `awaiting_pr_review` (com `pr_url:` preenchido na description).
7. **Humano aprova/mergeia o PR no GitHub** (gate humano).
8. **`kernl sweep`** detecta PR mergeado e fecha o épico-bead automaticamente (ver §6).

### 5.3 Merger agent prompt (em `orchestrator/internal/prompt/merger_prompt.go`)

Template renderizado com:
- `epic_id`, `epic_title`, `epic_branch`
- Lista ordenada (topologicamente) de `(child_bead_id, child_branch, child_worktree_path)`
- `base_branch` (master)
- Instruções concretas: `cd epic worktree`, `git checkout epic_branch`, loop de `git merge --no-ff`, resolver conflitos lendo markers, `git push`, `gh pr create`

Custo: ~100 LOC de prompt + ~30 LOC de render em Go.

**TODO follow-up explícito (não no MVP, mas em vista):** definir estratégia de contexto pro merger — que arquivos do repo o merger lê pra ser inteligente em conflitos. Candidatos: `AGENTS.md` do repo target, últimos N commits da master, plano original do épico (descrição do épico-bead), descrições dos filhos sendo mergeados. **Esta decisão fica documentada como item de design para a próxima iteração da spec do prompt**, não pro implementador da fase atual.

### 5.4 Política de conflito de merge

**MVP:** o merger agent **tenta resolver** conflitos (lê os marcadores `<<<<<<<`, decide, edita, `git add`, `git commit`). Se o agent não conseguir convergir em N tentativas (cap de follow-up) ou se o watchdog detectar estagnação → bead-épico vira `blocked`, com `merge_conflict_at: <branch>` na description. Humano resolve via git diretamente e re-roda `kernl epic merge <epic-id>` (subcomando dedicado a re-disparar o passo de integração).

**Fora do MVP (SotA — agente resolvedor especializado):** sub-bead dinâmico de "resolution_agent" com prompt especializado em git conflict resolution, validações estritas (testes têm que passar), múltiplas estratégias de resolução. Registrado na §10 como item futuro.

### 5.5 Testes pós-merge antes do PR

**Decisão MVP: NÃO rodar.** A CI do PR cobre isso quando humano aprova. Manter o merger focado em "merge + push + PR" reduz superfície de erro. Configurável depois (`kernl.yaml: epic_post_merge_command`).

### 5.6 `gh pr create` no fim

Disparado pelo merger agent dentro do prompt. Body auto-gerado: título do épico + lista de filhos mergeados (com IDs e títulos) + link pra spec se houver na description do épico.

### 5.7 Componentes novos

| Pacote | Responsabilidade | LOC estimado |
|---|---|---|
| `orchestrator/internal/merge/` | `MergeManager`: detecta condição "todos filhos done", dispara merger agent, transiciona estados. | ~150 |
| `orchestrator/internal/prompt/merger_prompt.go` | Template + render do prompt do merger. | ~130 (prompt + go) |
| Adição em `orchestrator/internal/worktree/` | Criar epic branch antes das worktrees dos filhos. | ~50 |
| Tests | Unit + integration pro fluxo "all children done → merger fires → PR opened". | ~200 |
| **Total incremental** | | **~530** |

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

```
para cada épico em bd list --status=awaiting_pr_review:
    pr_url = getMetadataField(epic.description, "pr_url")
    se pr_url vazio:
        skip (anomalia — logar warning)

    pr_state = gh pr view <pr_url> --json state,mergedAt,mergeCommit
    se pr_state.state == "MERGED":
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
  auto_interval_seconds: 60   # 0 = desabilitado
  github_token_env: GH_TOKEN  # se precisar de auth não-default
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
| W8 | **Delete completo do knots** (não dormante). | Decisão revisada nesta sessão: knots dormante apenas adia o problema. Delete tudo agora, simplifica o codebase, fecha o TODO "Remoção completa do knots". |
| W9 | Perfis customizáveis (`autopilot`/`semiauto`/etc.) deletados. | YAGNI no MVP. Voltam se aparecer caso de uso. |
| W10 | Migração big-bang em PR único. | kernl pré-MVP, sem produção a proteger; phased não ganha nada. |
| W11 | Spec única cobrindo workflow + auto-merge + sweep + migração. | Acoplamento conceitual alto demais pra separar; ler 3 docs pra entender 1 coisa é pior. |

---

## 8. Plano de migração (big-bang em PR único)

### 8.1 Pré-condição

- Rename foolery→kernl já consumado em `orchestrator/internal/` e `orchestrator/cmd/` (confirmado nesta sessão).
- Spec aprovada (este documento).

### 8.2 Mudanças concretas no PR `workflow/kernl-spec-migration`

**Deletes:**
- `orchestrator/internal/backend/knots.go`, `knots_test.go` — knots inteiro fora.
- `orchestrator/internal/backend/state_machine.go`: deleta `profileConfig`, `builtinProfiles`, `agentOwners`, `semiautoOwners`, `normalizeProfileID`, `descriptorFromProfileConfig`, `initBuiltinWorkflows`, `BuiltinProfileDescriptor`, `resolveWorkflow`, `canonicalTransitions`, `buildStates`, `filterTransitions`, `deriveWorkflowStructureFromConfig`, `stepOwnerKind`. Sobra: ~150 LOC com o novo modelo simples.
- `orchestrator/internal/backend/port.go`: encolhe `WorkflowDescriptor` (tira `Owners`, `QueueActions`, `ActionStates`, `ReviewQueueStates`, `HumanQueueStates`, `StateOwners`, `FinalCutState`, `Mode`, etc.).
- `orchestrator/internal/backend/factory.go`: remove qualquer roteamento pra knots; simplifica pra `bd` único.
- Constantes `agent_owners`/`semiauto_owners`/`autopilot*` e perfis correlatos.

**Novos:**
- `orchestrator/internal/workflow/status.go` — `IssueStatus`, `AgentState`, `KernlCustomStatuses`, métodos semânticos.
- `orchestrator/internal/workflow/description.go` — `getMetadataField`, `addMetadataField`, helpers tipados (`ParseAgentFields`, `FormatAgentFields`, etc.).
- `orchestrator/internal/workflow/ensure_custom.go` — `EnsureCustomStatuses(beadsDir)` idempotente com cache em memória + sentinel.
- `orchestrator/internal/merge/manager.go` — MergeManager.
- `orchestrator/internal/prompt/merger_prompt.go` — template do prompt do merger.
- `orchestrator/internal/sweep/sweep.go` — lógica do `kernl sweep`.
- `orchestrator/cmd/kernl/sweep.go` — subcomando Cobra (ou equivalente do framework do kernl).
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
  - `deferred` → `closed` (com reason "deferred") ou trata como `blocked` dependendo do contexto do teste
  - `abandoned` → `closed` (com reason "abandoned")

**Specs:**
- `orchestrator/specs/00-architecture.md` — limpa referência a `foolery.go` stale (linha 522), atualiza diagrama do workflow, remove menções a perfis.
- `orchestrator/specs/backend/backend.md` — atualizado pra refletir o novo write path (status built-in/custom + description), preservando provenance `[source: foolery/src/...]` apenas onde o behavior contract herdado segue valendo. Adiciona seção "Custom statuses" e "Description-field contracts".
- `orchestrator/specs/orchestration/orchestration.md` — atualiza o lifecycle do épico (com merger + sweep), o diagrama de transições.
- `orchestrator/specs/prompt/prompt.md` — adiciona seção do merger prompt + TODO do contexto-aware.
- (Outros specs tocados conforme inspeção.)

**Docs:**
- `TODOS.md` — fecha "Definir workflow próprio do kernl" + "Remoção completa do knots do codebase".

### 8.3 Gates de qualidade

1. **962 unit tests passam** (após adaptação das assertions sobre status names).
2. **Passo A do MVP** (`kernl epic run <single-bead>` contra bd CLI real) passa pelo menos até o ponto que dependia desta migração.
3. **`bd doctor`** (do bd) não reporta status inválidos no `.beads/` do próprio kernl.
4. **`go vet ./...` + linters** limpos.

### 8.4 Risco residual

- **Description-field parsing edge cases:** linhas com `:` no valor, escapes, BOM, etc. Mitigação: copiar a regex/string handling do gastown 1:1 — eles já bateram nesses casos.
- **EnsureCustomStatuses race condition:** dois processos kernl tentando registrar simultaneamente. Mitigação: gastown usa `mkdir` atômico + sentinel file na pasta `.beads/`; copiar o padrão.
- **Knots delete pode quebrar import paths em locais não-listados.** Mitigação: `grep -r "internal/backend/knots" orchestrator/` antes de mergear; deletar/refatorar consumers órfãos.
- **Fixtures com estado legacy que era semanticamente único** (`implementation_review` vs `shipment_review`): podem precisar de remodelagem por teste. Mitigação: revisar cada fixture caso a caso na PR review; alguns testes podem ser repensados ou removidos.

---

## 9. Critérios de sucesso (verificáveis)

1. `kernl epic run <epic-id>` contra bd 1.0.4 real avança o estado do bead sem `validation failed`.
2. Após todos os filhos terminarem, o merger agent é despachado automaticamente e merge das worktrees-filhas → epic branch acontece.
3. Em caminho-feliz (sem conflito), o `gh pr create` roda e o épico fica em `awaiting_pr_review` com `pr_url:` na description.
4. `kernl sweep --dry-run` lista épicos com PR mergeado sem efetuar.
5. `kernl sweep` (sem `--dry-run`) fecha filhos e épico após PR aprovado/mergeado em master.
6. `bd ready` no kernl não retorna nenhum bead com status legacy do foolery (smoke-check: a refatoração foi completa).
7. Zero referências a knots em `orchestrator/internal/` e `orchestrator/cmd/`.
8. Conflito de merge intratável faz o épico ir pra `blocked`, e `kernl epic merge <epic-id>` re-dispara o passo após resolução manual.

---

## 10. Fora de escopo (backlog explícito)

- **Agente resolvedor de conflito especializado** (Self-Healing Merges do SotA) — sub-bead dinâmico com prompt especializado, múltiplas estratégias, validação rigorosa.
- **AST-aware paralelism detection** — detectar dependências lógicas entre beads (não só file-overlap) antes de despachar.
- **GitHub webhook pra sweep** — substituir polling por webhook quando o kernl evoluir pra modo multi-user ou tiver server público.
- **Perfis customizáveis** (`autopilot`/`semiauto`/etc.) — voltam se aparecer caso de uso real, com design renovado.
- **Estados de gate humano por bead** (`awaiting-gate` do gastown) — entram quando o conceito de gates discretos por bead aparecer.
- **AgentState extra** (`escalated`, `paused`, `idle`, `patrolling`, `nuked`) — entram conforme features que os justifiquem.
- **Testes pós-merge antes do PR** — configuráveis depois (`epic_post_merge_command`).
- **Limpeza automática de worktrees** (TODO existente em `TODOS.md`) — independente desta spec.

---

## 11. Próximos passos

1. Esta spec é commitada em `docs/`.
2. Usuário revisa.
3. Após aprovação, escolher caminho de implementação:
   - **(a) `vc-plan`** — produz plano de implementação em tasks.
   - **(b) `vibe-engineering-mastery`** — caminho pesado com reviews CEO/eng/devex antes do plano, dado que a migração tem escopo arquitetural significativo.
4. Plano de implementação → `vc-convert-plan-to-beads` → execução pelo próprio kernl (dogfooding na primeira oportunidade real após o MVP rodar).

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
