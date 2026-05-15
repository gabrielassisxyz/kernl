# Engineering Review — Núcleo de Orquestração do Kernl (MVP)

**Data:** 2026-05-14
**Skill:** `vc-plan-eng-review` (via `vibe-engineering-mastery`)
**Entrada revisada:** `docs/2026-05-14-orchestration-nucleo-mvp-brainstorm-spec.md` + `docs/STRATEGY.md`
**Codebase:** `~/repositories/_cloned/foolery-go` (será renomeado — ver Issue 6)
**Status:** 10 issues decididas, 0 não-resolvidas. Voz de fora rodada (subagente), 2 tensões resolvidas pelo usuário.

---

## Step 0 — Scope Challenge

O `glob` `docs/DESIGN.md` não casou, mas o brainstorm spec **é** o design doc (tem problema, constraints, decisões, riscos) e foi usado como fonte da verdade.

### 🚨 Correção factual — a premissa central do spec estava errada

O spec afirmava que o `foolery-go` era "um backend funcional que lê `.beads/`, despacha agentes e roda take loops" e que o único bloqueio era "nunca foi rodado". A exploração mostrou que a verdade é mais cara:

**O `foolery-go` é uma biblioteca de peças de motor bem testadas, sem produto montado.**

- A camada HTTP é **100% stub** — todo handler em `internal/api/` devolve `struct{}{}` vazio. `grep` confirma zero referências de `api/` para `orchestration/`, `dispatch/`, `terminal/`, `backend/`.
- O endpoint SSE roteado (`api/streams.go`) é um stub quebrado. O SSE real (`session/sse.go` → `SessionConnectionManager.ServeSSE`) existe, é correto, tem buffer + fan-out — mas **nada o constrói nem o roteia**.
- **Nenhum entrypoint monta o motor.** `grep` por quem constrói `SessionConnectionManager` / roda o take loop fora de testes: só as próprias definições. `main.go` monta o router → handlers stub.
- **Zero testes de integração.** Os 791 `func Test` são herméticos.
- **Não existe executor de grafo/épico.** `orchestration/hierarchy.go` só monta árvore pai/filho pra exibição. O motor roda *um take loop por beat*. A capacidade que o spec chama de "motivo de existir do foolery-go" não está implementada.

### `fix_plan.md` é fonte de estado confiável (o spec errou ao descartá-lo)

O cabeçalho diz "zero .go files", mas os campos `Status:` por gap foram mantidos: **42 COMPLETED / 31 PENDING**. A seção 2.8 (API & SSE) lista os Gaps 8.1–8.4 como **PENDING** — o buraco da API é trabalho conhecido, não regressão.

### Demais checagens do Step 0

- **Complexity check: disparou** — e a premissa #1 do spec era falsa. Levou a um STOP + AskUserQuestion (resolvido na Issue 1).
- **Distribution:** cola + GUI são artefatos novos; para um MVP local single-user, pipeline CI/release → `NOT in scope`.
- **Search check:** worktrees + SSE `net/http` = Layer 1 (boring tech), conhecimento in-distribution basta.
- **TODOS.md:** não existia.

---

## Decisões (10 issues)

### Architecture (Section 1)

**Issue 1 — Quem orquestra o épico, e qual o papel do foolery-go.**
Achado: o `swarm_orchestrator.py` (558 linhas Python, commitado no mesmo dia, **nunca rodou**) já fazia worktree+paralelo+merge sem o foolery-go; e o foolery-go não tem executor de grafo. Tensão: o paralelismo, "motivo de existir do foolery-go", já estava resolvido por fora dele.
**Decisão:** tudo em Go, um binário, sem cola Python separada. O `EpicExecutor` vive dentro do (renomeado) foolery-go. O `swarm_orchestrator.py` é descartado como base.

**Issue 2 — Estrutura de pacotes e a git-agnosticidade.**
Esclarecimento: "módulo separado" vs "dentro do foolery-go" eram falsos opostos — ambos são in-process Go, sem API, sem latência. O risco real era o *núcleo* perder a git-agnosticidade.
**Decisão:** o núcleo (`orchestration`/`terminal`/`dispatch`/`session`) continua git-agnóstico como **invariante de pacote**. `internal/worktree` (novo) é o único pacote que conhece git. `internal/epic` (novo, o `EpicExecutor`) é a costura. `internal/api` é fino — HTTP só pra GUI. Disparo de épico via subcomando CLI.

**Issue 3 — Modelo de estado do EpicExecutor + "continuar de onde parou".**
Achados: `retake` e `watchdog` (COMPLETED) não resolvem recuperação de crash; o estado de bead vive no `bd` (durável), o de sessão só em memória. O motivo do AGENTS.md ser enfático sobre "bd é o único store" é evitar o bug de I/O de N escritas no mesmo `.json` — escritas particionadas não têm esse problema.
**Decisão:** **Opção D-completo, storage SQLite, dentro do MVP.** Run-state = `bead→worktree path` (1:1, estável) + registros por `(bead, estado)` `{agent id, session id, status}`. Detecção de interrupção via estado do bead no `bd`. Resume via `opencode run -s <session_id>` **não-interativo** + nudge. Persistir os registros de agente também fecha o buraco de cross-agent review através de crash. Sessões persistem como **dado** (no DB do opencode), não como processo vivo — zero custo ocioso.

**Issue 4 — Feed de dados da GUI.**
**Decisão:** novo endpoint SSE de **épico** (`/api/epics/{id}/events`), alimentado pelo `EpicExecutor` (eventos: bead-state-changed, session-started, session-error, wave-advanced), reusando o padrão de buffering/fan-out do `SessionConnectionManager`. Drill-down por-sessão (o `ServeSSE` existente) incluído no MVP, **prioridade mínima**.

**Issue 5 — Localização das worktrees.**
**Decisão:** fora do repo, raiz configurável no `kernl.yaml` — `~/.kernl/worktrees/<epic-id>/<bead-id>/`. Branch `kernl/<bead-id>`. Persistem pós-épico (D4 do spec — merge manual). `git worktree prune` + limpeza é manual/pós-MVP.

**Issue 6 — Rename foolery→kernl e estrutura de repo.**
"foolery" está em 109 arquivos Go. O rename é 100% mecânico, mas tem blast radius.
**Decisão:** o rename é **um commit isolado, estrutural-puro, feito ANTES do trabalho de feature** — com gate explícito: **791 testes verdes antes E depois; se quebrar, é a tarefa inteira até voltar verde** (refinamento da voz de fora #7). O repo vira `kernl` (reusa o `.git` do foolery-go); o bloco de orquestração vive sob `orchestrator/`; módulo `github.com/<user>/kernl`, pacotes sob `orchestrator/internal/...`. O rename inclui os arquivos de knots (renomeados, ainda dormentes — não é fork). Nome próprio/marca, se quiser, vem depois como camada de display, sem tocar no código.

### Code Quality (Section 2)

**Issue 7 — EpicExecutor: falha terminal de um bead-filho.**
**Decisão:** **fail-fast** — para de disparar ondas novas, irmãos independentes em voo terminam, épico pausa em `blocked`, GUI sinaliza, humano intervém. Uma falha terminal É um ponto de julgamento (alinha com a estratégia).

**Issue 8 — DRY: prompt de nudge.**
**Decisão:** **builder único de prompt de nudge, parametrizado pela causa** (`turn-ended` | `resumed-after-interruption`). O núcleo da mensagem fica num lugar só.

**Nota de plano (sem AUQ):** os pacotes novos seguem as convenções do `AGENTS.md` — arquivos <500 linhas, funcs 4-40 linhas, fail-loud com marcador (`KERNL DISPATCH FAILURE`), fakes nomeados nos boundaries. Os handlers stub atuais que devolvem `struct{}{}` silenciosamente **violam** fail-loud — os handlers reais não podem repetir isso.

### Test Review (Section 3)

**Issue 9 — Caminho ponta-a-ponta: suíte de integração codificada ou smoke manual.**
**Decisão:** TDD pros unit tests herméticos (inline, sempre). Suíte `//go:build integration` **codificada**, sequenciada — Passo A e Passo B são literalmente os dois primeiros testes de integração. Não é fase deferida; é a forma codificada do que o spec já planeja. Ressalva: testes que tocam opencode custam tokens/tempo pra *rodar* (opt-in/manual), mas isso não atrasa o MVP — não bloqueiam `go test ./...`.

Projeção de cobertura (nível-componente; o diagrama file-level vem com o plano do `vc-writing-plans`):

```
COMPONENTE NOVO                   UNIT (hermético)        INTEGRAÇÃO (-tags=integration)
orchestrator/internal/epic        DAG, ready-set,         [→INT] Passo B: épico paralelo,
  (EpicExecutor)                  fail-fast, exclusão       ordem do grafo respeitada
orchestrator/internal/worktree    add/remove/prune c/      [→INT] ciclo real de worktree
  (WorktreeManager)               fake de exec.Command
orchestrator/internal/app         wiring/assembly         —
SQLite run-state store            CRUD c/ :memory:        [→INT] resume-após-crash
thin API + epic-SSE               handlers c/ fakes       [→INT] emissão de eventos SSE
resume logic                      detecção via estado bd  [→INT] Passo A: 1 bead real
cross-agent review (buraco        exclusão lê registros   [→INT] review exclui o
  fechado pela Issue 3)           persistidos               implementador após crash
```

### Performance (Section 4)

**Issue 10 — Cap de concorrência na execução paralela.**
**Decisão:** semáforo no dispatch de onda do `EpicExecutor`; `max-concurrent-beads` configurável no `kernl.yaml`, default 5. O problema-mor do `GO_PORT.md §2` (rewrite de JSON compartilhado) já foi neutralizado pela escolha de SQLite WAL na Issue 3.

---

## OUTSIDE VOICE (subagente)

Subagente com contexto fresco, 12 findings. Triagem:

### Tensões cross-model — resolvidas pelo usuário

- **#10 — Re-questionar adotar o foolery-go.** A voz de fora: a premissa de D1 ("já funcional") foi invalidada pela review. **Resolução do usuário: re-confirmar o foolery-go como base** — conscientemente. Racional: as partes testadas (take loop, dispatch, máquina de estados, adapter `bd`, `SessionConnectionManager` — 42 gaps COMPLETED, 791 testes) são head start real; o que falta (executor, API, worktree) é greenfield de qualquer jeito.
- **#5 — Resume (Decisão 3) é over-engineered pra um MVP que nunca rodou.** **Resolução do usuário: manter a Decisão 3 exatamente como travada, sem re-sequenciar.** Nota factual (não crítica): a suíte de integração do resume depende de execução-de-épico existir, então cai naturalmente depois por dependência.

### Lacunas aditivas — incorporadas

- **#1 — MVP sem escopo/prazo coerentes.** Resolvido: MVP re-derivado (abaixo); milestone atualizado na `STRATEGY.md` para condição-de-conclusão + data-alvo 2026-05-16.
- **#3 — Encanar os stubs da API não tinha dono.** Resolvido: vira item de trabalho explícito na Fase 2 ("ligar a camada API ao motor pela 1ª vez").
- **#7 — Rename-first sem gating dito.** Resolvido: Issue 6 refinada com o gate "791 verdes antes E depois".
- **#11 — Métrica "paralelismo realizado" sem suporte no plano.** Resolvido: o `EpicExecutor` registra/expõe dados de paralelismo (item na Fase 3).
- **#12 — Estado "blocked" da Decisão 7 dava em beco sem saída.** Resolvido: o caminho de saída do "blocked" é re-rodar `kernl epic run` (humano corrige, re-dispara) — mesmo mecanismo do resume.
- **#9 — Critério de sucesso #5 degrada pra "metade" se as skills quebrarem.** Resolvido: dito explicitamente — se as skills `vibe-*` quebrarem, o fallback `.beads/` à mão prova só a metade de execução.

### Já pego pela review

- **#6 — `opencode run -s` não-testado** — a review já sinalizou (confidence 7/10). Elevado para **tarefa de de-risking explícita e cedo** (Fase 1).
- **#8 — Tradução formato `.beads/`** — já anotado como TODO. Novo lar: `orchestrator/internal/epic`. Ver TODOS.md.
- **#4 — knots** — a review já decidiu (Issue 1: dormante). O rename mecânico inclui os arquivos de knots.

### Correção de enquadramento

- **#2 — O `EpicExecutor` não é "uma costura", é a feature-núcleo não-construída.** Aceito — é o maior componente greenfield do MVP. Registrado com o peso certo na Fase 3.

**Cross-model tension:** 2 pontos de tensão, ambos resolvidos pelo usuário (ver acima). Demais findings da voz de fora foram aditivos (incorporados) ou já-pegos.

---

## MVP re-derivado (Fases)

Agrupado por dependência; o sequenciamento fino é trabalho do `vc-writing-plans`.

- **Fase 0 — Rename** (commit isolado, estrutural-puro, primeiro; gate: 791 verdes antes E depois). foolery→kernl em tudo; repo vira `kernl`, bloco sob `orchestrator/`, módulo `github.com/<user>/kernl`.
- **Fase 1 — De-risking.** Verificar semântica de `opencode run -s <session_id>`. Passo A: disparar 1 bead real → opencode real spawna → take loop avança.
- **Fase 2 — Montagem do produto.** `orchestrator/internal/app` (monta o motor). Ligar a camada API ao motor pela 1ª vez — fino. Subcomando `kernl epic run <id>`.
- **Fase 3 — Feature-núcleo: execução paralela de épico.** `orchestrator/internal/worktree`. `orchestrator/internal/epic` (`EpicExecutor`: lê `.beads/`, DAG, ready-set, semáforo `max-concurrent-beads`, fail-fast, registra dados de paralelismo). Tradução `.beads/`↔adapter `bd` se preciso. Passo B: 1 épico real (parent + ≥2 filhos c/ deps) em paralelo.
- **Fase 4 — Observabilidade.** Endpoint SSE de épico. GUI mínima (`web/`): beads+estado, sessões ativas, flag de erro. Drill-down por-sessão (prioridade mínima).
- **Fase 5 — Estado durável & resume.** Store SQLite. Lógica de resume. Caminho de saída do "blocked" = re-rodar `kernl epic run`.
- **Cross-cutting:** unit tests herméticos TDD inline em todo pacote novo; suíte `-tags=integration` codificada.

---

## What already exists (reuso vs reconstrução)

| Componente | Estado | Plano |
|---|---|---|
| Take loop / máquina de estados (`terminal/`, `orchestration/workflow.go`) | COMPLETED, testado hermético | **Reusa.** É o valor único do foolery-go. |
| Dispatch + rotação de agente + exclusão cross-agent (`dispatch/`) | COMPLETED | **Reusa.** Cross-agent review é decisão firme (specs/00-architecture.md:168). |
| Adapter `bd` CLI (`backend/bdcli.go`) | COMPLETED | **Reusa.** |
| `SessionConnectionManager` + `ServeSSE` (buffer + fan-out) | COMPLETED, correto | **Reusa.** Roteia pro drill-down por-sessão; o padrão de fan-out é reusado pelo SSE de épico. |
| Follow-up loop (`terminal/followup.go`) | COMPLETED | **Reusa.** O `BuildTakeLoopFollowUpPrompt` é generalizado pelo builder único (Issue 8). |
| `retake` / `watchdog` | COMPLETED | Existem, mas **não** resolvem recuperação de crash — não confundir com resume. |
| Camada HTTP API (`internal/api/`) | **100% stub** | **Constrói.** Ligar ao motor pela 1ª vez (Fase 2). |
| Endpoint SSE | **Stub quebrado** | **Constrói.** Roteia pro `ServeSSE` + novo endpoint de épico. |
| Entrypoint que monta o motor | **Não existe** | **Constrói.** `orchestrator/internal/app` (Fase 2). |
| Executor de grafo/épico | **Não existe** | **Constrói.** `orchestrator/internal/epic` — é o `WaveExecutor` do `GO_PORT.md §4.8`, projetado-não-implementado (Fase 3). |
| Worktree / git | **Não existe** (por design, foolery-go era git-agnóstico) | **Constrói.** `orchestrator/internal/worktree` (Fase 3). |
| knots / kno | COMPLETED parcial, 101 refs / 14 arquivos | **Dormante.** Zero gaps knots novos; registrar só repos beads. |

---

## NOT in scope

- **Frontend Vue completo** — descartado; a GUI do MVP é `web/` HTML/JS estático.
- **Loop agêntico de integração/review/merge/PR** — review/merge é fallback manual no MVP (spec D4). É o bloco imediatamente seguinte.
- **Multi-épico e conflitos entre épicos paralelos** — um épico por vez.
- **Multi-repo** — um repo registrado.
- **Desenvolvimento de knots** — dormante; nenhum gap knots novo.
- **Remoção de knots do codebase** — 101 refs / 14 arquivos; é um projeto próprio, sem ganho pro MVP. Ver TODOS.md.
- **Pipeline CI/release** dos artefatos novos (cola/GUI) — MVP local single-user.
- **Limpeza automática de worktree** — manual no MVP. Ver TODOS.md.
- **Os 5 caminhos de entrada do `mermaid-diagram.js`** — MVP usa 1.
- **Resume "D-full" via adapter interativo** (`opencode serve`) — o D-completo do MVP usa `opencode run -s` não-interativo, que é mais barato. O adapter interativo (Gap 4.3 PENDING) fica fora.

---

## Failure modes

| Codepath novo | Falha realista em produção | Coberto por teste? | Error handling? | Usuário vê? |
|---|---|---|---|---|
| `EpicExecutor` dispara onda | Um filho falha terminalmente (cap de follow-up, pool esgotado) | Sim (`-tags=integration` + unit) | Sim — fail-fast (Issue 7) | Sim — GUI flag `blocked` |
| `WorktreeManager` cria worktree | `git worktree add` falha (path existe, repo sujo) | Sim (unit c/ fake) | **Deve** — fail-loud `KERNL DISPATCH FAILURE` | Sim — via SSE de épico |
| Resume após crash | `bd` diz estado ativo mas worktree foi corrompido/removido | Sim (`-tags=integration`) | **Deve** — detectar worktree ausente, re-criar ou sinalizar | Sim |
| Resume via `opencode run -s` | opencode não suporta `-s` como assumido (confidence 7/10) | De-risking Fase 1 | N/A até verificar | — |
| SQLite run-state | escrita falha / arquivo corrompido | Sim (unit) | **Deve** — fail-loud; `bd` continua sendo a verdade pra estado de bead | Sim |
| SSE de épico | canal de eventos satura / cliente lento | Sim (unit) | Buffer `maxConnectionBuffer=5000` já existe | Degrada, não quebra |
| EpicExecutor lê `.beads/` | formato do `vc-convert-plan-to-beads` ≠ esperado pelo adapter `bd` | — | Camada de tradução (ver TODOS.md) | Sim — fail-loud no parse |

**Gap crítico:** nenhum com `sem teste + sem error handling + falha silenciosa`. O `opencode run -s` não-verificado é o maior risco aberto — mitigado pelo de-risking da Fase 1.

---

## Worktree parallelization strategy

| Fase / workstream | Módulos tocados | Depende de |
|---|---|---|
| Fase 0 — Rename | repo inteiro | — |
| Fase 1 — De-risking (`opencode -s`, Passo A) | — (verificação + run manual) | Fase 0 |
| Fase 2 — App assembly + API wiring | `orchestrator/internal/app`, `orchestrator/internal/api` | Fase 0 |
| Fase 3a — WorktreeManager | `orchestrator/internal/worktree` | Fase 0 |
| Fase 3b — EpicExecutor | `orchestrator/internal/epic` | Fase 3a, Fase 2 |
| Fase 4 — SSE de épico + GUI | `orchestrator/internal/api`, `web/` | Fase 3b |
| Fase 5 — SQLite + resume | `orchestrator/internal/epic`, store novo | Fase 3b |

**Parallel lanes:**
- `Lane A: Fase 0 → Fase 1` (gate sequencial — tudo depende do rename)
- `Lane B: Fase 2` (independente da Fase 3a — app/api não tocam worktree)
- `Lane C: Fase 3a` (WorktreeManager — independente da Fase 2)
- Depois: `Fase 3b` (espera B + C) → `Fase 4` e `Fase 5` em paralelo (ambas dependem de 3b, módulos diferentes — `web/`+`api` vs `epic`+store)

**Execution order:** Fase 0 → Fase 1 → (Lane B + Lane C em paralelo) → Fase 3b → (Fase 4 + Fase 5 em paralelo).
**Conflict flag:** Fase 3b, Fase 4 e Fase 5 todas tocam `orchestrator/internal/epic` — Fase 4 e Fase 5 em paralelo têm risco de conflito de merge nesse pacote. Coordenar ou sequenciar 4→5.

---

## Completion summary

- **Step 0: Scope Challenge** — escopo travado: reusar o motor portado, construir só a fatia PENDING que o núcleo precisa, knots dormante, port completo abandonado. Premissa factual do spec corrigida.
- **Architecture Review** — 6 issues encontradas e decididas (Issues 1–6).
- **Code Quality Review** — 2 issues encontradas e decididas (Issues 7–8); 1 nota de convenção.
- **Test Review** — diagrama de cobertura (nível-componente) produzido; decisão de suíte de integração codificada (Issue 9).
- **Performance Review** — 1 issue encontrada e decidida (Issue 10).
- **NOT in scope:** escrito.
- **What already exists:** escrito.
- **TODOS.md updates:** 3 itens propostos ao usuário (ver `TODOS.md`).
- **Failure modes:** 0 gaps críticos (`sem teste + sem handling + silencioso`); maior risco aberto = `opencode run -s` não-verificado, mitigado pelo de-risking Fase 1.
- **Outside voice:** rodou (subagente); 12 findings; 2 tensões cross-model resolvidas pelo usuário.
- **Parallelization:** 3 lanes paralelas possíveis (B+C, depois 4+5); resto sequencial por dependência.
- **Lake Score:** 10/10 recomendações da review foram na direção da opção mais completa/correta (não houve atalho escolhido).

## Unresolved decisions

Nenhuma. As 10 issues foram decididas; as 2 tensões da voz de fora foram resolvidas pelo usuário.
