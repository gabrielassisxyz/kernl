# Test Plan — Núcleo de Orquestração do Kernl (MVP)

Gerado por `/vc-plan-eng-review` em 2026-05-14
Codebase: `~/repositories/_cloned/foolery-go` (será renomeado para `kernl`)
Convenção: testes herméticos em `*_test.go` (default `go test ./...`); integração em `//go:build integration` (`go test -tags=integration ./...`, opt-in/manual).

## Componentes/rotas afetados

- `orchestrator/internal/epic` (EpicExecutor, novo) — DAG, ready-set, dispatch de onda, fail-fast, semáforo, registro de paralelismo.
- `orchestrator/internal/worktree` (WorktreeManager, novo) — `git worktree add/remove/prune`.
- `orchestrator/internal/app` (assembly, novo) — monta o motor a partir das peças.
- Store SQLite de run-state (novo) — `bead→worktree` + registros por `(bead,estado)`.
- `orchestrator/internal/api` — handlers reais (hoje stubs) ligados ao motor; novo endpoint SSE de épico `/api/epics/{id}/events`; roteamento do `ServeSSE` por-sessão.
- Subcomando CLI `kernl epic run <id>`.
- Lógica de resume (detecção via estado `bd` + `opencode run -s` + nudge).

## Interações-chave a verificar

- **DAG / ready-set:** deps satisfeitas → filho entra no ready-set; deps pendentes → não entra; ordem do grafo respeitada.
- **Dispatch paralelo:** filhos sem dep entre si rodam concorrentes; semáforo `max-concurrent-beads` limita o spawn simultâneo.
- **Cross-agent review:** o agente que fez `implementation` é excluído do pool de `implementation_review`; fallback pro mesmo agente com banner se o pool esvazia.
- **Resume:** crash mid-`implementation` → bead em estado ativo no `bd` → resume da sessão interrompida com `-s` + nudge; crash no gap (bead avançou, sem agente disparado) → dispatch fresco respeitando exclusão cross-agent.
- **Fail-fast:** falha terminal de um filho → nenhuma onda nova; irmãos independentes em voo terminam; épico em `blocked`.
- **SSE de épico:** `EpicExecutor` emite bead-state-changed / session-started / session-error / wave-advanced; cliente recebe o agregado.
- **Worktree:** create; reuso do worktree sujo no resume; a worktree é per-bead, compartilhada pelos agentes sequenciais do bead.

## Edge cases

- DAG com diamante de dependências; ciclo no grafo (deve ser detectado e recusado).
- `git worktree add` falha (path já existe, repo sujo, disco cheio).
- Resume quando o worktree foi removido/corrompido fora do Kernl.
- `bd` diz estado X mas o run-state SQLite diz Y → `bd` ganha.
- Escrita no SQLite falha / arquivo corrompido por crash.
- `opencode run -s` com session-id inválido ou sessão que o opencode já descartou.
- Épico mais largo que `max-concurrent-beads` (ondas escalonadas).
- Cap de follow-up estourado dentro do take loop de um filho.

## Critical paths (têm que funcionar)

- **Passo A** (Fase 1, primeiro teste `-tags=integration`): `kernl epic run` com 1 bead real → opencode real spawna → take loop avança o bead → estado final correto no `bd`.
- **Passo B** (Fase 3, segundo teste `-tags=integration`): 1 épico real (parent + ≥2 filhos com deps) → filhos sem dep entre si rodam em paralelo → ordem respeita o grafo → todos os beads chegam a estado terminal.
- **Resume** (`-tags=integration`): matar o processo mid-épico → re-rodar `kernl epic run` → beads done são pulados, bead interrompido é resumido, épico completa.
- **Cross-agent review após crash** (`-tags=integration`): crash entre `implementation` e `implementation_review` → no restart, o review exclui o implementador persistido.
- **GUI** (Fase 4): a GUI mínima consome o SSE de épico e mostra beads+estado, sessões ativas, flag de erro.

## Notas

- Testes de integração tocam opencode/bd/git reais → custam tokens/tempo, rodam opt-in/manual, nunca no `go test ./...` default.
- `opencode run -s` é assumido (confidence 7/10) — a Fase 1 de de-risking verifica o flag real antes de a lógica de resume depender dele.
- O diagrama de cobertura file-level (por função/branch) será produzido quando o `vc-writing-plans` gerar o plano com as funções concretas.
