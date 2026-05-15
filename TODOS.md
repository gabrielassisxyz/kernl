# TODOS

Itens diferidos, capturados com contexto suficiente pra serem retomados meses depois.

---

## Remoção completa do knots do codebase

- **What:** Avaliar e executar a remoção completa do backend `knots`/`kno` do codebase do orchestrator.
- **Why:** O knots tem 101 referências em 14 arquivos não-teste — está acoplado em `backend/port.go`, `factory.go`, `registry.go`, `state_machine.go`, `terminal/manager.go` (o `SessionEntry` tem `knotsLeaseFields`), `takeloop.go` (rollback tem modo knots/beads), `orchestration/workflow.go`, `dispatch/forensics*`. Mantê-lo dormante funciona, mas deixa superfície morta e acoplamento que complica leitura e mudanças futuras.
- **Pros:** Superfície de código menor; menos acoplamento; o núcleo fica mais simples de entender pra contribuidores open-source.
- **Cons:** É um projeto de refactor próprio (14 arquivos, lógica de rollback dual-mode, factory de roteamento); zero ganho funcional pro MVP; competiria direto com a feature-núcleo se feito agora.
- **Context:** Decisão da Issue 1 da eng review (`docs/reviews/vc-plan-eng-review-2026-05-14.md`): o MVP mantém o knots **dormante** — zero gaps knots novos, registrar só repos beads no runtime, o `factory` nunca roteia pro `KnotsBackend`. O código knots já portado fica compilado e parado. Esta remoção é a evolução pós-MVP disso.
- **Depends on / blocked by:** Confirmar que o knots nunca será usado como memory manager em nenhum repo registrado no Kernl. Se houver qualquer chance de uso, não remover.

---

## Limpeza automática de worktree

- **What:** Automação de limpeza das worktrees em `~/.kernl/worktrees/` — `git worktree prune` + remoção das worktrees de épicos já mergeados ou abandonados. Possivelmente um subcomando `kernl worktree clean` ou um hook pós-merge.
- **Why:** A Issue 5 da eng review decidiu que as worktrees **persistem pós-épico** (merge/review é manual no MVP — spec D4) e a limpeza é manual. Com o uso, worktrees órfãs se acumulam em disco indefinidamente.
- **Pros:** Disco não enche de worktrees mortas; menos manutenção manual; estado de `~/.kernl/worktrees/` reflete só trabalho ativo/pendente.
- **Cons:** Precisa de um conceito confiável de "épico concluído" pra saber o que é seguro remover — e esse conceito pertence ao loop de integração/merge, que é o bloco *seguinte* ao MVP.
- **Context:** Decisão da Issue 5 (`docs/reviews/vc-plan-eng-review-2026-05-14.md`): worktrees fora do repo, raiz configurável `~/.kernl/worktrees/<epic-id>/<bead-id>/`, persistem pós-épico pra merge manual. Limpeza explicitamente manual/pós-MVP.
- **Depends on / blocked by:** O loop agêntico de integração/review/merge (o bloco imediatamente seguinte ao MVP) precisa definir quando um épico está "concluído" — sem isso, não há critério seguro de remoção.

---

## Automação de onboarding & comandos do projeto

- **What:** Um wrapper que encapsula os comandos do projeto (build/test/run/`kernl doctor`/lint) E reduz o TTHW pra <5min. Forma: script bonitinho, subcomando, ou docker-compose — **não** Makefile. Inclui a visão de longo prazo: uma skill no Claude Code que configura tudo, ou um comando CLI que deixa o ambiente quase pronto.
- **Why:** Hoje a persona decora comandos crus (`go run`, `go test`, `golangci-lint`, `air`) e o setup do zero leva 15-30min. O MVP aterrissa conscientemente em "Needs Work" (5-10min); "open source desde o princípio" precisa de Champion/Competitive tier (<5min) pra outro dev não desistir no setup.
- **Pros:** TTHW competitivo; a persona não precisa lembrar dos comandos; onboarding de open-source deixa de ser Red Flag.
- **Cons:** A automação de setup precisa lidar com pré-reqs externos (bd CLI, opencode) que ela não controla; docker-compose adiciona uma dependência de runtime.
- **Context:** Decisões 0C, F1 e Pass 6 da DevEx review (`docs/reviews/vc-plan-devex-review-2026-05-14.md`). O `kernl doctor` (Fase 2) já cobre a *checagem* de pré-reqs; este TODO é a *automação* da instalação/configuração e dos comandos do dia-a-dia.
- **Depends on / blocked by:** O MVP rodando (Passo A/B) e o `kernl doctor` existindo — a automação se apoia neles.

---

## CONTRIBUTING.md

- **What:** Guia de contribuição para humanos: setup do ambiente, como rodar testes (hermético e `-tags=integration`), convenções do `AGENTS.md`, processo de PR/branch.
- **Why:** A STRATEGY diz "open source desde o princípio". Sem CONTRIBUTING, outro dev não sabe como contribuir — e o `AGENTS.md` é briefing de *agente*, não de *humano*. O README quickstart (deliverable cross-cutting do MVP) cobre "como rodar", não "como contribuir".
- **Pros:** Reduz a barreira pra contribuições externas; deixa explícitas as convenções que hoje só vivem no `AGENTS.md`.
- **Cons:** Custo de manutenção (mais um doc pra manter alinhado); pouco valor enquanto o repo é privado/só-você.
- **Context:** Pass 7 da DevEx review (`docs/reviews/vc-plan-devex-review-2026-05-14.md`). O MVP adiciona `LICENSE` (MIT, preservando copyright do acartine); CONTRIBUTING é o passo seguinte de community.
- **Depends on / blocked by:** O repo ir a público no GitHub + `LICENSE` commitado.

---

## Definir workflow próprio do kernl (substituir state machine herdada do foolery)

- **What:** O kernl herdou de `acartine/foolery` (referência TS) uma state machine com ~10 estados próprios — `ready_for_implementation`, `implementation`, `implementation_review`, `review`, `shipment_review`, `shipped` — armazenados diretamente no campo `status` dos beads. Bd pré-1.0 aceitava qualquer string ali; **bd 1.0.4 valida `status` contra um set restrito** (`open`, `in_progress`, `blocked`, `closed`, etc.) e rejeita os legados. Precisa decidir o modelo do workflow do kernl daqui pra frente e refatorar 27 arquivos para alinhá-lo com o que o bd aceita.
- **Why:** Sem isso, integration tests não rodam (Passo A, Passo B, resume tests), o magical moment do PLAN.md (`kernl epic run` end-to-end com workers paralelos contra bd real) não é validável, e toda chamada de write path (`bd update`, `bd close` com state custom) falha contra um bd 1.0+. O kernl está **code-complete mas não operacionalmente validado** contra o bd atual — esse é o gap que separa "tem código" de "MVP funciona".
- **Pros:** Destrava validação end-to-end do pipeline; alinha kernl com a evolução do bd 1.0+; oportunidade de planejar o workflow próprio (não apenas copiar o foolery), refletindo o que o kernl realmente quer modelar (talvez menos estados, ou estados diferentes — ex: o conceito de `shipment_review` herdou semantics que o kernl pode não precisar).
- **Cons:** Refactor substancial — 27 arquivos afetados, incluindo specs (`specs/backend/backend.md`, etc.). Decisões de modelagem ainda não tomadas:
  - (A) **Rebrand 1:1** mapeando ready_for_implementation→open, implementation→in_progress, etc. — fácil, mas perde a nuance entre `implementation_review` e `shipment_review`, que não têm equivalente em bd.
  - (B) **Workflow state em metadata** — bd `status` fica sempre num valor válido (open/in_progress/closed), e o estado fino do kernl vai pra `metadata.kernl_workflow_state`. Preserva semântica, refactor médio (~15 arquivos), mas adiciona uma camada de indireção.
  - (C) **Workflow inteiramente novo do kernl** (não copiar foolery) — mais trabalho mas alinhado com "o que o kernl precisa" em vez de "o que o foolery tinha". Recomendado dada a dívida.
- **Context:** Status legados são herança direta do foolery (TS): https://github.com/acartine/foolery/blob/main/TAXONOMY.md e https://github.com/acartine/foolery/blob/main/docs/SETTINGS.md. Arquivos kernl que referenciam `ready_for_implementation` (lista mecânica, não estratégia): `internal/backend/{state_machine,factory,knots,bdcli,dto}.go` + seus `*_test.go`, `internal/dispatch/{dispatch,forensics}_test.go`, `internal/orchestration/{sorting,workflow}.go` + tests, `internal/retake/retake.go` + test, `internal/terminal/{followup,takeloop_rollback,takeloop}_test.go`, `internal/epic/resume_test.go`, `internal/app/driver_test.go`, `internal/integration/harness.go`, `internal/integration/passo_a_test.go`, `cmd/kernl/bead_test.go`, ambas as fixtures `internal/integration/testdata/beads-*/.beads/issues.jsonl`, e os specs (`00-architecture.md`, `backend/backend.md`, `orchestration/orchestration.md`, `prompt/prompt.md`). Os 962 unit tests passam porque usam fakes que aceitam qualquer string — só integration tests (que invocam bd CLI real) expõem o drift. Descoberto na branch `fix/integration-test-setup` ao tentar `bd init --from-jsonl` no harness; sintoma: `validation failed: invalid status: ready_for_implementation`. Branch tem o setup do harness consertado (`bd init` + import) — o que falta é o workflow refactor.
- **Depends on / blocked by:** Decisão estratégica de qual modelo de workflow o kernl quer. Recomendação: passar por `vibe-engineering-mastery` ou STRATEGY/brainstorm dedicado dada a magnitude — não fazer cego em uma sessão de coding. O TODO "Remoção completa do knots do codebase" (acima) interage: o `knots.go` também usa o status legado, então a remoção do knots pode antecipar parte do refactor (ou ser uma boa hora pra fazer tudo junto).

---

## Instrumentar as 3 métricas restantes da STRATEGY

- **What:** Hooks de captura para as 3 métricas-chave da STRATEGY que o MVP não instrumenta: "intervenções fora de gate por épico" (a leading), "épicos concluídos sem resgate manual", "ideias→épico executado/mês".
- **Why:** A STRATEGY define 4 métricas-chave; o MVP (eng review Fase 3) instrumenta só "paralelismo realizado". Sem as outras 3 — especialmente a leading — não dá pra saber se o gargalo (a tese central do produto) realmente se moveu.
- **Pros:** Fecha o loop estratégia↔medição; "intervenções fora de gate" mede o gargalo direto que justifica o produto.
- **Cons:** "Intervenções fora de gate" exige primeiro **definir "gate" e "intervenção" como conceitos de runtime** — o MVP não tem isso. "Ideias→épico/mês" cruza a fronteira pra camada de planejamento (skills vibe-*).
- **Context:** Pass 8 da DevEx review (`docs/reviews/vc-plan-devex-review-2026-05-14.md`). A métrica mais barata — "épicos concluídos sem `blocked`" — foi considerada pro MVP mas o usuário optou por só paralelismo.
- **Depends on / blocked by:** Definir "gate" e "intervenção" como conceitos de runtime do `EpicExecutor`; o loop de gates de julgamento (track da STRATEGY) estar mais maduro.

---

## Property-based race test pro trigger "todos filhos awaiting_integration"

- **What:** Adicionar property test (rapid ou testing/quick) que simula timing aleatório de transições simultâneas de filhos pra estressar o single-flight lock do EpicExecutor/MergeManager. Complementa o test determinístico já planejado.
- **Why:** A decisão D11=A da eng review 2026-05-15 (`docs/reviews/vc-plan-eng-review-2026-05-15.md`) entrega cobertura determinística sólida (N goroutines com sync.WaitGroup, sem timing aleatório). Property test cobre permutações que não foram pensadas explicitamente. Race conditions em scheduling de agent são exatamente onde defeitos sutis nascem.
- **Pros:** Confidence extra na correção do trigger; explicita o contrato; pega edge cases que tabelas estáticas não cobrem.
- **Cons:** Property tests podem flake (timing-dependent); lentos em CI; precisam de seed reproduzível pra debug.
- **Context:** Decisão D11 (`vc-plan-eng-review-2026-05-15.md`). Trigger lógico vive em `orchestrator/internal/merge/manager.go` (a criar). Spec referência: `docs/2026-05-15-kernl-workflow-brainstorm-spec.md` §5.2.
- **Depends on / blocked by:** Implementação do MergeManager + EpicExecutor wiring do PR de migração do workflow.

---

## Batch heartbeats em memória no AgentStateStore

- **What:** Otimização: acumular updates de heartbeat/follow_up_count/watchdog state em memória; flush atomic write a cada N segundos (ex: 5-10) ao invés de a cada heartbeat individual.
- **Why:** A decisão D12=A da eng review 2026-05-15 entrega atomic write tempfile+rename por heartbeat (~1ms em SSD) — bom default. Em ambientes IOPS-restritos (HDD, NFS, contêineres com fsync barreirado) o custo cresce linearmente com workers ativos × frequência de heartbeat. Batch reduz drasticamente.
- **Pros:** Reduz IOPS drasticamente em volumes altos; permite kernl rodar bem em ambientes restritos; transparente pra leitores (read consulta mem-buffer + disk fallback).
- **Cons:** Janela de perda em crash maior (até N segundos de heartbeats voam); complica leitura (precisa olhar mem-buffer + disk); semantics de "freshness" mais difícil de explicar.
- **Context:** Decisão D12 (`docs/reviews/vc-plan-eng-review-2026-05-15.md`). Só vale a pena instrumentar **depois** que houver evidência real de problema de IOPS — endurece em resposta a dor, não antecipadamente. Arquivo afetado: `orchestrator/internal/workflow/agent_state_store.go` (a criar).
- **Depends on / blocked by:** Observabilidade do AgentStateStore real em uso; idealmente uma métrica de IOPS exposta no `kernl serve`.

---

## Subcomando `kernl epic abort`

- **What:** Novo subcomando `kernl epic abort <epic-id>` que dá caminho limpo pra cancelar épico em andamento. Operações: (1) sinalizar workers ativos pra terminar, (2) deletar worktrees do épico em `~/.kernl/worktrees/<epic-id>/`, (3) deletar branches `feat/<epic-id>` e `feat/<child-ids>` não-pushadas, (4) marcar filhos+épico `closed` com `--reason=aborted`, (5) limpar JSON local em `~/.kernl/state/<bead-id>.json` de cada bead afetado.
- **Why:** Outside voice da eng review 2026-05-15 (TT4) flagou o gap: operador querendo abortar épico em andamento não tem caminho limpo no MVP. Workaround manual via `bd close --reason="aborted"` por bead + cleanup manual de worktrees + branches é feio e propenso a deixar lixo. DX ruim, especialmente porque o caso "abortei porque o épico tomou rumo errado" é provável de aparecer cedo no uso real.
- **Pros:** First-class workflow pra cancellation; limpeza determinística; ~150 LOC sobre infra existente (WorktreeManager, BdCliBackend, AgentStateStore); orthogonal ao resto do workflow.
- **Cons:** +1 subcomando no escopo; "abortar enquanto worker tá ativo" tem nuance (esperar terminar vs kill -9 vs intermediário); decisão de "abortar com PR já aberto" também precisa pensar.
- **Context:** Decisão TT4=B da eng review 2026-05-15 (`docs/reviews/vc-plan-eng-review-2026-05-15.md`). Outside voice (subagent independente) levantou; review interno aceitou diferir. Implementação reusa: `WorktreeManager.Remove`, `BdCliBackend.Close`, `AgentStateStore.Purge`, plus git operations diretas pra branches locais.
- **Depends on / blocked by:** PR de migração do workflow + MergeManager terminado e estável (define o espaço de estados que `abort` precisa cobrir).
