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

## Instrumentar as 3 métricas restantes da STRATEGY

- **What:** Hooks de captura para as 3 métricas-chave da STRATEGY que o MVP não instrumenta: "intervenções fora de gate por épico" (a leading), "épicos concluídos sem resgate manual", "ideias→épico executado/mês".
- **Why:** A STRATEGY define 4 métricas-chave; o MVP (eng review Fase 3) instrumenta só "paralelismo realizado". Sem as outras 3 — especialmente a leading — não dá pra saber se o gargalo (a tese central do produto) realmente se moveu.
- **Pros:** Fecha o loop estratégia↔medição; "intervenções fora de gate" mede o gargalo direto que justifica o produto.
- **Cons:** "Intervenções fora de gate" exige primeiro **definir "gate" e "intervenção" como conceitos de runtime** — o MVP não tem isso. "Ideias→épico/mês" cruza a fronteira pra camada de planejamento (skills vibe-*).
- **Context:** Pass 8 da DevEx review (`docs/reviews/vc-plan-devex-review-2026-05-14.md`). A métrica mais barata — "épicos concluídos sem `blocked`" — foi considerada pro MVP mas o usuário optou por só paralelismo.
- **Depends on / blocked by:** Definir "gate" e "intervenção" como conceitos de runtime do `EpicExecutor`; o loop de gates de julgamento (track da STRATEGY) estar mais maduro.
