# DevEx Review — Núcleo de Orquestração do Kernl (MVP)

**Data:** 2026-05-14
**Skill:** `vc-plan-devex-review` (via `vibe-engineering-mastery`)
**Entrada revisada:** `docs/2026-05-14-orchestration-nucleo-mvp-brainstorm-spec.md` + `docs/STRATEGY.md` + `docs/reviews/vc-plan-eng-review-2026-05-14.md` + `docs/reviews/vc-plan-eng-review-test-plan-2026-05-14.md`
**Codebase:** `~/repositories/_cloned/foolery-go` (será renomeado — Issue 6 do eng review)
**Modo:** DX POLISH
**Product type:** CLI Tool (primário) — binário orquestrador + subcomandos; API fina + GUI mínima de monitoramento como superfície secundária
**Status:** Step 0 completo (7 etapas) + 8 passes. 12 decisões de fricção tomadas, 3 TODOs registrados, 0 não-resolvidas. Outside voice: pulada (escolha do usuário).

---

## Developer Persona Card

```
TARGET DEVELOPER PERSONA
========================
Who:       Solo dev que forkou o foolery-go e quer montar sua própria
           orquestração multi-agente em Go. Foco principal do MVP é ele
           mesmo; o produto será open source pra outros devs depois.
Context:   Conhece Go, construiu o repo Kernl e as skills de planejamento,
           mas ainda está aprendendo as entranhas do motor herdado
           (take loop, dispatch, state machine, adapter bd) — "ainda não
           entendi o foolery por completo".
Tolerance: Alta pra fricção de setup (é o próprio projeto), mas baixa pra
           "coisas que não fazem sentido" — se o motor faz algo inesperado,
           precisa entender rápido ou perde tempo em debug.
Expects:   `go build`, subcomando funcional, config YAML com exemplos reais
           e comentados, logs que explicam o que o motor está fazendo.
```

Implicação central: a persona é, ao mesmo tempo, **autor e newcomer** do próprio motor. A DX precisa servir a **operação** (rodar épicos) E a **compreensão** (entender o que o foolery-go faz e como as peças encaixam). Foi isso que puxou o achado do glossário (Pass 4) e do vocabulário beat/bead (Pass 2).

---

## Developer Empathy Narrative

> *São 3 semanas depois do MVP. Eu volto pro repo `kernl` pra rodar um épico novo. Abro o `README.md` — 0 bytes. Ok, vou pelo que lembro. O eng review fala em `kernl epic run <id>`, então tento `go run ./cmd/kernl --help`. Não sei se existe help text — o `main.go` do foolery-go só sobe um servidor HTTP, não tem subcomando nenhum ainda.*
>
> *Preciso de um `kernl.yaml`. O único exemplo é o `foolery.yaml.example`: 6 linhas, `agents: {}`, `pools: {}`, `repos: []` — mapas vazios, zero comentário. Não lembro o shape de um agent nem de um pool. Vou ter que abrir `internal/config/` e ler o struct. 10 minutos perdidos só pra montar config.*
>
> *Agora preciso do `bd` instalado e do opencode configurado. O plano assume os dois, mas nada checa se estão lá. Rodo `kernl epic run` e — se `bd` não estiver no PATH — recebo um `DISPATCH FAILURE` em algum ponto fundo do dispatch, não um "instala o bd primeiro" no início.*
>
> *Disparo o épico. A GUI sobe num `web/` estático. Vejo beads e estados mudando. Funciona — mas um bead trava. O épico vai pra `blocked`. A GUI mostra a flag. E agora? Não lembro que "a saída do blocked é re-rodar `kernl epic run`" — isso está no eng review, não num lugar que eu veja na hora.*

**Status:** confirmada pelo usuário ("Bate — segue com esse entendimento"). O ponto central — o plano é forte em arquitetura/testes mas a superfície "como um humano opera isto" não tinha dono em nenhuma fase — virou a espinha dorsal desta review. As fricções F1–F6 e as confusões #1–#2 saíram daqui.

---

## Competitive DX Benchmark

```
COMPETITIVE DX BENCHMARK — orquestradores locais de agentes paralelos
======================================================================
Tool                  | TTHW          | Escolha de DX notável            | Fonte
Parallel Code         | ~2 min        | npx, cada agente em worktree     | github.com/johannesjo/parallel-code
Composio Agent Orch.  | ~5 min        | pré-reqs explícitos, 1 dashboard | github.com/ComposioHQ/agent-orchestrator
CLI Agent Orch. (AWS) | ~5-10 min     | MCP supervisor-worker, tmux      | aws.amazon.com/blogs/opensource
Superset / Conductor  | ~3 min        | app local, zero-config worktree  | superset.sh
KERNL (plano atual)   | ~15-30 min    | go build + bd + opencode +       | spec + eng review
                      | (∞ p/ novo)   |   kernl.yaml à mão + épico .beads/|
KERNL (pós-review)    | ~5-10 min     | kernl doctor + yaml comentado +  | esta review
                      |               |   README cross-cutting + epic list|
```

O diferencial dos concorrentes é **`npx` e tá rodando**. O Kernl tem uma cadeia mais pesada — mas é justificada: é um orquestrador de *épicos* (grafo de beads com dependências), não de tasks soltas. **Decisão do usuário (0C):** aterrissar em **Needs Work (5-10 min)** no MVP; TTHW <5min (setup skill / comando CLI que deixa quase pronto / docker-compose) é pós-MVP — virou TODO 1.

---

## Magical Moment Specification

**Momento mágico:** ver um épico rodando em paralelo — beads mudando de estado, agentes simultâneos, o grafo de deps sendo respeitado, o humano só olhando. É literalmente o "Passo B" do spec.

**Veículo de entrega escolhido (0D):** **os dois** — GIF vende, exemplo prova.
- **`examples/parallel-demo/`** — um épico empacotado no repo (parent + 2-3 filhos com deps já em `.beads/`) + `kernl epic run examples/parallel-demo`. Prova o motor na máquina do dev. Custo incremental baixo: o Passo B já constrói um épico real rodando.
- **GIF/asciinema no topo do README** — gravado da mesma execução. Fricção zero pra quem descobre o projeto; gancho de open-source.

Requisito de implementação: o exemplo empacotado precisa rodar opencode real (custa tokens/tempo) — documentar isso no README ao lado do comando.

---

## Developer Journey Map

```
STAGE          | DEVELOPER DOES                       | FRICTION POINTS              | STATUS
---------------|--------------------------------------|------------------------------|--------
1. Discover    | acha o repo kernl no GitHub          | README 0 bytes               | FIXED (0D: GIF + exemplo)
2. Install     | go build, instala deps, kernl.yaml   | F1 pré-reqs não checados     | FIXED (F1: kernl doctor)
               |                                      | F2 yaml.example vazio        | FIXED (F2: exemplo comentado + erro acionável)
3. Hello World | roda Passo A (1 bead real)           | F3 README/quickstart sem dono| FIXED (F3: deliverable cross-cutting)
4. Real Usage  | kernl epic run <id>                  | F4 de onde vem o <id>?       | FIXED (F4: kernl epic list no MVP)
5. Debug       | épico vai pra blocked, lê logs       | F5 saída do blocked invisível| FIXED (F5: blocked imprime o próximo passo)
               |                                      | F6 logs slog JSON ilegíveis  | FIXED (F6: output plain-text, JSON atrás de flag)
6. Upgrade     | —                                    | baixa fricção (MVP pré-1.0)  | OK (nota: rename foolery.yaml→kernl.yaml)
```

### Resoluções de fricção (F1–F6)

- **F1 — `kernl doctor` (preflight).** Subcomando que checa `bd` no PATH, opencode configurado, Go version, `kernl.yaml` válido — e diz exatamente o que falta. Roda também automaticamente no início de `kernl epic run`. Item de plano na Fase 2.
- **F2 — `kernl.yaml.example` rico + comentado + erro acionável.** Ship um exemplo completo (1 agent opencode preenchido, 1 pool, 1 repo registrado, cada campo comentado). MAIS: `kernl epic run` com config vazia/inválida falha loud nomeando o campo que falta e apontando pro `.example`. Item de plano na Fase 2.
- **F3 — README/quickstart é deliverable cross-cutting.** Igual aos unit tests no eng review: cada fase que toca um caminho operável atualiza a sua parte do README. Sem dono único, sem vácuo.
- **F4 — `kernl epic list` no MVP.** Subcomando que lista épicos disponíveis no `.beads/` (id, título, nº de filhos, estado). Alinha com o adapter `bd` que já existe. Item de plano na Fase 2.
- **F5 — o estado `blocked` imprime o próximo passo.** CLI e evento SSE carregam a ação: qual bead falhou, por quê, e "corrija e rode `kernl epic run <id>` de novo pra retomar". Item de plano na Fase 3/4.
- **F6 — output de operador plain-text, slog JSON atrás de flag.** `kernl epic run` imprime progresso human-readable por padrão (bead → estado, agente spawnado, onda avançada); `--log-format=json` / `KERNL_LOG_LEVEL=debug` revela o slog estruturado. Alinha com o próprio AGENTS.md ("plain text ONLY for user-facing output"). Item de plano na Fase 2.

---

## First-Time Developer Confusion Report

```
FIRST-TIME DEVELOPER REPORT
============================
Persona: solo dev que forkou o foolery-go, contexto parcial do motor
Attempting: rodar um épico do zero no Kernl

CONFUSION LOG:
T+0:00  Clono o repo. README tem conteúdo (GIF + quickstart). Sigo.        [resolvido por 0D/F3]
T+0:30  kernl doctor — bd ok, opencode ok, Go ok. Copio kernl.yaml.example. [resolvido por F1/F2]
T+1:30  Quero ver a GUI rodar — mas como? É servida pelo epic run?         [CONFUSÃO #1]
T+3:00  Épico rodando, beads acendendo. Momento mágico — funciona.
T+5:00  Épico chega ao fim. E agora? Onde está o código?                   [CONFUSÃO #2]
T+6:00  Vou caçar as worktrees na mão. Funciona, mas termina no escuro.
```

- **Confusão #1 — relação `kernl epic run` ↔ servidor/GUI.** Resolução (escolha do usuário: "definir e documentar"): **`kernl epic run` sobe o servidor HTTP/SSE+GUI embutido no mesmo processo e imprime a URL no startup** (`GUI em http://localhost:PORT`). Modelo de menor surpresa, alinha com "um binário" (Issue 1 do eng review). Definido no plano + README; sem novo item de trabalho.
- **Confusão #2 — o épico "termina no escuro".** Resolução (escolha do usuário): **fora de escopo** — o loop de integração/review/merge é explicitamente o bloco seguinte ao MVP (spec §8). Registrado em "NOT in scope". O MVP aceita conscientemente que o pós-épico não é guiado em runtime.

---

## Review Sections (8 passes)

Modo POLISH: todas as 8 passes avaliadas. Rating antes → depois dos fixes do Step 0.

### Pass 1 — Getting Started Experience: 2/10 → 7/10
**2/10** porque a persona do empraty narrative bate em fricção a cada etapa: README 0 bytes, sem quickstart, pré-reqs não checados, `yaml.example` de mapas vazios, sem descoberta de épico. TTHW Red Flag (~15-30min). **Fixes:** F1 (`kernl doctor`), F2 (yaml comentado + erro acionável), F3 (README cross-cutting), F4 (`kernl epic list`), 0D (GIF + exemplo empacotado). **7/10** — não é 10 porque TTHW <5min (setup skill / docker-compose) é explicitamente pós-MVP por escolha do usuário (0C → TODO 1). Aterrissa em "Needs Work", aceitável pro MVP com o caminho documentado.

### Pass 2 — API/CLI Design: 5/10 → 8/10
CLI: `kernl epic run` / `kernl epic list` / `kernl doctor` — verb-noun consistente, guessable, namespace `epic` claro. Defaults: F4 escolheu `kernl epic list` explícito. **5/10** inicial por causa do split de vocabulário **beat vs bead**: o codebase usa "beat" (`/api/beats`, `listBeatsHandler`, "beat id" no AGENTS.md) mas o domínio do Kernl usa "bead" (`.beads/`, CLI `bd`, STRATEGY inteira). Exploração do `specs/00-architecture.md` confirmou: **são a mesma entidade** — o `Backend` port é o adapter do CLI `bd` (`bd show <beatId>` é literalmente chamado); "beat" é o apelido temático-musical do autor do foolery (foolery/setlist/beat). **Resolução:** `beat → bead` entra **junto no rename da Fase 0**, sob o mesmo gate "791 verdes antes E depois". **8/10** — um vocabulário só, alinhado com o domínio.

### Pass 3 — Error Messages & Debugging: 6/10 → 8/10
O spec fail-loud do AGENTS.md (§2) é forte: nomeia a coisa que falta + a config exata que conserta + marcador greppable (`DISPATCH FAILURE`). O eng review já pegou que os handlers stub que devolvem `struct{}{}` violam isso. **6/10** porque a tabela de failure modes do eng review marca os codepaths novos (worktree, SQLite, parse de `.beads/`) como "Deve — fail-loud" mas só especifica o **marcador**, não o **conteúdo** — e os codepaths novos (`git worktree add` falha, `opencode run -s` inválido) não têm "config que conserta" óbvia. **Resolução:** **padrão de conteúdo pros codepaths novos** — todo erro fail-loud dos pacotes novos (`worktree`, `epic`, store, `api`) carrega problema + causa + fix acionável + (quando aplicável) próximo comando. Vira nota de convenção no plano, igual à nota do eng review sobre os handlers stub. F5 já abriu esse precedente pro estado `blocked`. **8/10**.

### Pass 4 — Documentation & Learning: 3/10 → 7/10
**3/10** inicial: README 0 bytes, `docs/activeContext.md` / `progress.md` / `glossary.md` / `architecture.md` todos 0 bytes; só `AGENTS.md` (briefing de agente, não getting-started) e `specs/00-architecture.md` têm conteúdo. **Fixes:** F3 (README/quickstart cross-cutting) + **glossary.md como deliverable cross-cutting** — cada fase que toca um termo novo (beat/bead, epic/wave, take loop, dispatch, knots dormante, retake/watchdog) o registra. Alto valor pra essa persona específica, que "ainda não entendeu o foolery por completo". **7/10** — não é 10 porque tutoriais/exemplos além do `examples/parallel-demo` ficam pós-MVP.

### Pass 5 — Upgrade & Migration Path: 6/10 → 6/10
MVP pré-1.0, single-user — há pouco a "atualizar de". Sem findings bloqueantes. **1 nota:** o commit de rename da Fase 0 deve mencionar explicitamente `foolery.yaml` → `kernl.yaml` e a mudança de module path no commit message / README, pra **você** (único "usuário" com config existente) não ser pego de surpresa. Sem pergunta — é execução, não decisão.

### Pass 6 — Developer Environment & Tooling: 7/10 → 7/10
`go test ./...`, `air` (dev), `golangci-lint run`, `go vet`, tags `//go:build integration` — toolchain Go padrão e sólido. **Achado:** não há runner que encapsule build/test/run/doctor/lint — a persona voltando em 3 semanas decora comandos crus. **Resolução:** **TODO pós-MVP** — explicitamente **não** Makefile (escolha do usuário: "não vejo ninguém mais usando makefile"); um wrapper/script bonitinho ou docker-compose. Virou TODO 1 (junto com a automação de onboarding). **7/10** mantido — o toolchain em si está bom; o runner é conforto.

### Pass 7 — Community & Ecosystem: 2/10 → 5/10
**2/10**: a STRATEGY diz "open source desde o princípio" mas o repo não tem `LICENSE` — sem licença, é legalmente "todos os direitos reservados". **Investigação:** o `foolery-go` herda de `acartine/foolery`, que é **MIT** (https://github.com/acartine/foolery/blob/main/LICENSE). O module path `github.com/gastownhall/foolery` no `foolery-go` é um upstream errado — a Fase 0 (rename do módulo) corrige de passagem. **Resolução:** Kernl `LICENSE` = **MIT** no MVP; o arquivo **precisa preservar o aviso de copyright do acartine** (requisito legal da MIT, já que o Kernl é derivado de `acartine/foolery`) — "inspiração no README" é um extra bom mas não substitui o crédito no `LICENSE`. CONTRIBUTING.md → TODO 2. **5/10** — LICENSE resolve o bloqueio legal; comunidade real (canais, exemplos, contributing) é pós-MVP por design.

### Pass 8 — DX Measurement & Feedback Loops: 4/10 → 4/10
A STRATEGY define 4 métricas-chave; o eng review (Fase 3) instrumenta **só "paralelismo realizado"**. As outras 3 — "intervenções fora de gate por épico" (a leading, a mais importante — mede o gargalo direto), "épicos concluídos sem resgate manual", "ideias→épico executado/mês" — não têm hook de captura. **Resolução:** **só paralelismo no MVP** (escolha do usuário) — "intervenções fora de gate" exige definir "gate" e "intervenção" como conceitos de runtime que o MVP não tem; diferir as 3 é honesto. Virou TODO 3. **4/10** mantido — é uma lacuna real e consciente, não um defeito do plano.

---

## NOT in scope

DX considerada e explicitamente diferida:

- **Confusão #2 — resumo de pós-épico guiado em runtime** — o "e agora" depois que o épico termina (onde estão as worktrees, como mergear) pertence ao loop de integração/review/merge, que é o bloco imediatamente seguinte ao MVP (spec §8).
- **TTHW < 5 min (Champion/Competitive tier)** — setup skill no Claude Code, comando CLI que deixa quase pronto, docker-compose. Pós-MVP — ver TODO 1. O MVP aterrissa conscientemente em "Needs Work" (5-10 min).
- **Runner de comandos do projeto** — wrapper/script/docker-compose pra build/test/run/doctor/lint. Pós-MVP — TODO 1.
- **CONTRIBUTING.md e canais de comunidade** — pré-publicação pública, não MVP — TODO 2.
- **Instrumentação das 3 métricas restantes da STRATEGY** — exige definir "gate"/"intervenção" como conceitos de runtime — TODO 3.
- **Tutoriais e exemplos além do `examples/parallel-demo`** — o exemplo empacotado é o suficiente pro momento mágico do MVP; mais exemplos são pós-MVP.
- **Playground/sandbox hospedado** — o veículo do momento mágico é exemplo local + GIF, não ambiente hospedado.

---

## What already exists (reuso de DX)

| Artefato de DX | Estado | Plano |
|---|---|---|
| `AGENTS.md` (foolery-go) | Conteúdo bom — stack, comandos, princípios, fail-loud spec | **Reusa.** É briefing de agente, não de humano — não substitui README/CONTRIBUTING. Renomear marcador `FOOLERY DISPATCH FAILURE` → `KERNL DISPATCH FAILURE` (Fase 0). |
| `specs/00-architecture.md` | Conteúdo autoritativo dos contratos | **Reusa.** Fonte pra o glossary (foi onde se confirmou beat=bead). |
| Spec fail-loud (`AGENTS.md §2`) | Forte — nomeia o que falta + config que conserta + marcador greppable | **Reusa e estende.** Pass 3: padrão de conteúdo pros codepaths novos. |
| `foolery.yaml.example` | 6 linhas, mapas vazios, zero comentário | **Substitui.** F2: `kernl.yaml.example` rico e comentado. |
| `README.md` | **0 bytes** | **Constrói.** F3: cross-cutting; 0D: GIF no topo. |
| `docs/glossary.md`, `activeContext.md`, `progress.md`, `architecture.md` | **0 bytes** | `glossary.md`: **constrói** (cross-cutting, Pass 4). Os outros: memory bank de sessão, fora do escopo desta review. |
| Endpoint SSE (`session/sse.go` — `SessionConnectionManager`) | COMPLETED, correto (eng review) | **Reusa.** Alimenta a GUI; o `kernl epic run` o serve embutido (Confusão #1). |
| Toolchain Go (`go test`, `air`, `golangci-lint`, tags de integração) | Padrão, sólido | **Reusa.** Runner que encapsula é TODO 1. |
| Licença | `acartine/foolery` é MIT; `foolery-go` sem LICENSE próprio | **Constrói.** Kernl `LICENSE` = MIT preservando copyright do acartine (Pass 7). |

---

## DX Scorecard

```
+====================================================================+
|              DX PLAN REVIEW — SCORECARD                            |
+====================================================================+
| Dimension            | Score  | Prior  | Trend  |
|----------------------|--------|--------|--------|
| Getting Started      |  7/10  |  2/10  |  +5 ↑  |
| API/CLI/SDK          |  8/10  |  5/10  |  +3 ↑  |
| Error Messages       |  8/10  |  6/10  |  +2 ↑  |
| Documentation        |  7/10  |  3/10  |  +4 ↑  |
| Upgrade Path         |  6/10  |  6/10  |   = →  |
| Dev Environment      |  7/10  |  7/10  |   = →  |
| Community            |  5/10  |  2/10  |  +3 ↑  |
| DX Measurement       |  4/10  |  4/10  |   = →  |
+--------------------------------------------------------------------+
| TTHW                 | 5-10min| 15-30m |   ↑    |
| Competitive Rank     | Needs Work (escolha consciente p/ MVP)      |
| Magical Moment       | designed via GIF no README + examples/parallel-demo |
| Product Type         | CLI Tool (primário) + API/GUI secundária    |
| Mode                 | DX POLISH                                   |
| Overall DX           |  6.5/10|  4/10  |  +2.5 ↑|
+====================================================================+
| DX PRINCIPLE COVERAGE                                              |
| Zero Friction        | covered (F1/F2/F3 — não Champion, é escolha)|
| Learn by Doing       | covered (examples/parallel-demo + glossary) |
| Fight Uncertainty    | covered (F5 blocked + Pass 3 conteúdo de erro)|
| Opinionated + Escape | covered (defaults + --log-format flag)      |
| Code in Context      | covered (exemplo empacotado, não hello-world)|
| Magical Moments      | covered (0D — GIF + exemplo)                |
+====================================================================+
```

Dimensões abaixo de 6 — **Community (5)** e **DX Measurement (4)** — não são defeitos do plano: são escopo conscientemente diferido pelo usuário (TODOs 2 e 3). Nenhuma dimensão é falha não-endereçada.

## DX Implementation Checklist

```
DX IMPLEMENTATION CHECKLIST
============================
[~] Time to hello world < target (5-10 min — Needs Work tier, escolha consciente)
[ ] Installation: kernl doctor (preflight) — Fase 2
[ ] kernl.yaml.example rico + comentado + erro acionável em config vazia — Fase 2
[ ] First run produz output human-readable (plain-text, JSON atrás de flag) — Fase 2
[ ] Magical moment: GIF no README + examples/parallel-demo + kernl epic run examples/parallel-demo
[ ] Todo erro fail-loud dos pacotes novos: problema + causa + fix + próximo comando
[ ] Estado `blocked` imprime o próximo passo (CLI + SSE)
[ ] kernl epic list — descoberta de épico — Fase 2
[ ] beat → bead no rename da Fase 0 (junto com foolery → kernl)
[ ] README/quickstart como deliverable cross-cutting (cada fase atualiza)
[ ] glossary.md como deliverable cross-cutting
[ ] LICENSE = MIT preservando copyright do acartine
[ ] kernl epic run sobe servidor/GUI embutido + imprime a URL — definido + documentado
[ ] Rename Fase 0 menciona foolery.yaml → kernl.yaml + module path no commit/README
[ ] Funciona em CI/CD: já coberto (go test hermético, integração opt-in — eng review)
```

---

## TODOS.md updates

3 itens propostos ao usuário, todos aprovados (ver `TODOS.md`):

1. **Automação de onboarding & comandos do projeto** — wrapper/script/docker-compose + visão de setup skill, alvo TTHW <5min. Pós-MVP.
2. **CONTRIBUTING.md** — guia de contribuição humano, pré-publicação pública.
3. **Instrumentar as 3 métricas restantes da STRATEGY** — intervenções fora de gate, épicos sem resgate, ideias→épico/mês.

---

## Completion Summary

- **Step 0: DX Investigation** — 7 etapas completas. Persona card (autor + newcomer do próprio motor), empathy narrative (confirmada), competitive benchmark (Needs Work tier escolhido), magical moment (GIF + exemplo), mode (POLISH), journey trace (F1–F6 resolvidos), confusion report (#1 definido, #2 fora de escopo).
- **8 passes** — todas avaliadas (modo POLISH, anti-skip). 4 dimensões subiram (Getting Started +5, Documentation +4, API/CLI +3, Community +3), 1 +2 (Error Messages), 3 estáveis (Upgrade, Dev Env, DX Measurement — as duas últimas baixas por escopo diferido consciente).
- **Achados novos das passes** — beat/bead (Pass 2), padrão de conteúdo de erro (Pass 3), glossary cross-cutting (Pass 4), LICENSE MIT + proveniência acartine (Pass 7).
- **Outside voice** — pulada (escolha do usuário).
- **TODOS.md** — 3 itens, todos aprovados.
- **Decisões tomadas** — 12 de fricção/achado, todas resolvidas pelo usuário via AskUserQuestion.
- **Overall DX** — 4/10 → 6.5/10. O salto concentra-se onde a alavanca era maior: a superfície "como um humano opera isto" passou a ter dono (cross-cutting), e os subcomandos operacionais (`doctor`, `epic list`) entraram no plano.

## Unresolved decisions

Nenhuma. As 12 decisões de fricção/achado foram resolvidas pelo usuário; os 3 TODOs foram aprovados; a confusão #2 e o tier de TTHW foram conscientemente diferidos.

---

**Sources (competitive benchmark):**
- [Parallel Code — github.com/johannesjo/parallel-code](https://github.com/johannesjo/parallel-code)
- [Composio Agent Orchestrator](https://github.com/ComposioHQ/agent-orchestrator)
- [AWS CLI Agent Orchestrator](https://aws.amazon.com/blogs/opensource/introducing-cli-agent-orchestrator-transforming-developer-cli-tools-into-a-multi-agent-powerhouse/)
- [Superset](https://superset.sh/)
- [acartine/foolery — LICENSE (MIT)](https://github.com/acartine/foolery/blob/main/LICENSE)
