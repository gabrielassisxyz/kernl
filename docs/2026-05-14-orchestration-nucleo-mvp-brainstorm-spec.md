# Brainstorm Spec — Núcleo de Orquestração do Kernl (MVP)

**Data:** 2026-05-14
**Bloco:** Núcleo de orquestração ("bloco zero" do Kernl)
**Prazo do MVP:** 2026-05-15, ~almoço
**Status:** design aprovado — pronto para `vc-plan`
> **Archival note (2026-05-18):** This is a historical brainstorm spec from early
> MVP planning. Some references it contains (Bubble Tea as TUI option, Memory Bank,
> `kno` as a CLI tool) have since been superseded. See root `AGENTS.md` and
> `docs/VISION.md` for current architecture.

---

## 1. Resumo em um parágrafo

O núcleo do Kernl é a orquestração multi-agente. Ele não é um terreno vazio: o ponto de
partida concreto é o `foolery-go` (`~/repositories/_cloned/foolery-go`), um port em Go —
linguagem do Kernl — do `foolery` original. O `foolery-go` já é um backend funcional
(~31k linhas, compila limpo, 922 testes herméticos passando) que lê `.beads/`, despacha
agentes e roda take loops. O bloqueio real do usuário não é "falta código" — é que o
`foolery-go` **nunca foi rodado**: nunca saiu dos testes herméticos para execução real.
O MVP é a **última milha**: fazer planejamento → beads → execução rodar de ponta a ponta
numa fatia vertical fina, com um épico real executado em paralelo, até amanhã no almoço.

## 2. Contexto e estado atual

- **`ideas.md` foi corrigido** nesta sessão: a constraint "vault Obsidian como fonte da
  verdade + sync bidirecional" está **descartada** (era design do `nexus`); o modelo de
  persistência novo é decisão *downstream* deste bloco.
- **Camada de planejamento já existe como skills:** `vibe-chaos-to-concept`
  (ideate→brainstorm→plan) e `vibe-engineering-mastery` (strategy→reviews→writing-plans→
  Yegge Loop→task breakdown→`vc-convert-plan-to-beads`). Rodam hoje, dentro do Claude Code.
  São skills **novas e ainda não testadas em uso real**.
- **Camada de execução já existe:** `foolery-go` — o "Foolery Executor" do
  `mermaid-diagram.js`. `fix_plan.md` dentro do `foolery-go` está **desatualizado** (foi
  escrito quando havia zero arquivos `.go`); não é fonte confiável de estado. O confiável:
  compila, 922 testes passam — mas testes são herméticos (fakes, não tocam CLIs/processos
  reais).
- **`foolery-go` é git-agnóstico:** zero referências a worktree, branch ou PR. O modelo
  dele é spawnar um agente CLI dentro de um `repoPath` e monitorar a sessão/estado do bead.
  Quem faz git é o agente, via prompt.
- **Fluxo revisado pelo usuário:** o planejamento já gera em semi-formato de beads, então
  há **um** Yegge loop (não dois): planejamento → Yegge loop → aprovação → cria os beads
  respeitando 100% o planejamento → execução.

## 3. Escopo do MVP e critérios de sucesso

**O que o MVP é** — um workflow de orquestração que roda de ponta a ponta numa fatia
vertical fina:

> tarefa real → `vibe-engineering-mastery` (fluxo completo) → 1 Yegge loop → aprovação →
> `vc-convert-plan-to-beads` → `.beads/` → **`foolery-go` executa o épico com tasks em
> paralelo** → GUI mínima mostra quem faz o quê e sinaliza erros.

O MVP usa de propósito o caminho **pesado** (`vibe-engineering-mastery`) — o objetivo é
estressar a pipeline mais pesada e as skills novas de uma vez.

**Critérios de sucesso (verificáveis):**

1. `foolery-go` sobe na máquina (`go run ./cmd/foolery`) e serve API + SSE.
2. Configurado: `foolery.yaml` com **1 harness de agente** (OpenCode), `bd` instalado,
   1 repo registrado.
3. **De-risking (passo A):** dispara 1 bead real → `foolery-go` spawna um agente real →
   take loop avança o bead. Prova que o executor sai do papel.
4. **Alvo real (passo B):** um épico (parent + ≥2 filhos com dependências) é executado —
   filhos sem dependência entre si rodam **em paralelo**, a ordem respeita o grafo.
5. O épico veio do fluxo de planejamento real (skills `vibe-*`), não montado à mão —
   *exceto* se a skill quebrar; aí o `.beads/` é montado à mão como fallback e o passo 4
   ainda conta.
6. GUI mínima: lista beads + estado, mostra sessões ativas, sinaliza agente com erro.
7. É possível mandar um agente "continuar de onde parou" (via take/follow-up do
   `foolery-go`, ou disparo manual).

**Por que épico em paralelo e não 1 task:** se o MVP fosse executar uma task por vez, não
precisaria do `foolery-go` — `subagent-driven-development` (superpowers) já faz dispatch
paralelo + worktrees + code-reviewer. O motivo de existir do `foolery-go` é executar
épicos (grafo de beads com dependências) com paralelismo; o MVP precisa demonstrar isso.

## 4. Arquitetura do loop

```
[1] Tarefa real
      │  Claude Code, hoje
      ▼
[2] vibe-engineering-mastery  ──►  plano em semi-formato beads
      │  1 Yegge loop ──► aprovação do usuário
      ▼
[3] vc-convert-plan-to-beads  ──►  .beads/  (1 épico = parent + N filhos c/ deps)
      │
      ▼
[4] COLA (peça nova)  ──►  cria 1 git worktree+branch por bead-filho
      │                    e dispara o épico no foolery-go via API
      ▼
[5] foolery-go executa  ──►  sessões-filho em paralelo (onde as deps permitem),
      │                      cada uma com seu worktree como repoPath,
      │                      take loop + monitoramento por filho
      ▼
[6] GUI mínima (HTML/JS + SSE)  ──►  beads+estado, sessões ativas, flag de erro
      │
      ▼
[7] Fim do épico  ──►  fallback manual no MVP (ver Decisão D4)
```

**Fronteiras do bloco (núcleo de orquestração):** `[4]` cola, `[5]` `foolery-go`, `[6]`
GUI. `[1]`–`[3]` são a camada de planejamento (skills, já existem) — o núcleo *consome* o
`.beads/` que elas produzem. `[7]` é manual no MVP.

### 4.1 A cola (`[4]`)

Peça **nova**, mínima. Mora no repo `kernl`, **não** dentro do `foolery-go`.

- Lê o `.beads/` do épico; para cada bead-filho: `git worktree add` numa branch própria.
- Mapeia bead-filho → worktree path; dispara o épico no `foolery-go` via API passando cada
  `repoPath`.
- Reusa o `repoPath` que o `foolery-go` já tem — **nenhuma mudança de git dentro do
  `foolery-go`**.
- Pode precisar de uma camada fina de tradução se a saída do `vc-convert-plan-to-beads`
  não bater com o formato que o adapter `bd` do `foolery-go` espera.
- Linguagem (Go ou shell): decisão do `vc-plan`.

### 4.2 A GUI (`[6]`)

`web/` standalone, HTML/JS/CSS puro, **sem relação com o frontend Vue original** do
`foolery`.

- Consome o endpoint SSE que o `foolery-go` **já expõe**.
- Mostra: lista de beads + estado, sessões ativas (quem faz o quê), flag visual de agente
  com erro.
- É um painel de monitoramento descartável/evoluível — não é o frontend do produto.

## 5. Sequência de execução: A → B (de-risking)

O MVP é a Abordagem B (loop completo fino), construída fazendo A primeiro como passo de
de-risking. Se o tempo acabar, A sozinho já é um marco usável.

**Passo A — fumaça (provar o executor; sem cola, sem skills):**

1. Instalar `bd` CLI (≥ 1.0.4).
2. Escrever `foolery.yaml` mínimo: 1 pool com 1 harness (OpenCode), 1 repo registrado.
3. `go run ./cmd/foolery` — servidor sobe, API + SSE respondem.
4. Criar 1 bead à mão; disparar take; ver OpenCode spawnar de verdade; bead avança.
5. **Saída:** "como rodar" documentado + lista crua do que quebrou.

**Passo B — loop completo:** a cola `[4]`, as skills `[1]`–`[3]` rodando de verdade,
execução do épico `[5]`, GUI `[6]`.

## 6. Decisões

| ID | Decisão | Razão |
|----|---------|-------|
| D1 | `foolery-go` é **base/reaproveitamento + ajuste**, não consumo externo nem reescrita do zero | É a língua do Kernl (Go) e já é funcional; ajustar é mais rápido que reescrever |
| D2 | MVP = Abordagem B via A | B é o workflow que o usuário quer; A isola o maior risco (nunca rodou) primeiro |
| D3 | Isolamento: **1 worktree por bead-filho + cola fina**; `foolery-go` não muda | Paralelismo e isolamento reais sem adicionar git dentro do `foolery-go` |
| D4 | Review da execução: **só no fim do épico**; no MVP, **fallback manual** — o usuário dispara manualmente um agente para revisar+mergear os worktrees | O loop agêntico completo de integração é uma segunda orquestração com gate de julgamento; construí-lo bem compete com fazer o épico #1 rodar |
| D5 | GUI: HTML/JS/CSS puro consumindo o SSE existente | Mais leve que frontend completo (Vue 3 + Nuxt — VISION §12 — vem pós-MVP); reusa infra SSE já existente |
| D6 | 1 Yegge loop, não 2 | O planejamento já gera em semi-formato de beads (mudança do usuário) |

## 7. Riscos e fallbacks

| Risco | Fallback |
|-------|----------|
| `foolery-go` nunca rodou — bootstrap/config pode quebrar | Passo A isola isso primeiro; é o de-risking |
| Skills `vibe-*` são novas, podem quebrar | Montar o `.beads/` à mão; passo B (execução do épico) ainda conta |
| Saída do `vc-convert-plan-to-beads` ≠ formato que o adapter `bd` espera | Camada fina de tradução na cola |
| Dispatch paralelo de filhos (scene) pode não funcionar em runtime | Passo A testa 1 bead; se scene falhar, rodar filhos sequencialmente e ainda provar o grafo |
| SSE pode não emitir eventos úteis pra GUI | GUI cai para polling da API REST |

## 8. Fora de escopo (backlog — blocos não descartados, só não agora)

- Frontend Vue completo do produto.
- **O loop agêntico completo de integração/review/merge/PR** (orquestrador-agente revisa
  branches → dispacha fix agents → julga "ok" → mergeia worktrees → reviewer do PR único →
  fixes → abre PR). É o **bloco imediatamente seguinte** ao MVP.
- Multi-épico e conflitos entre épicos paralelos.
- Multi-repo.
- Encerramento do fluxo com dev session notes (`activeContext.md`, `progress.md`) / `bd remember` / atualização de `AGENTS.md`.
- Os 5 caminhos de entrada do `mermaid-diagram.js` (o MVP usa 1).
- Demais áreas do `ideas.md`: wiki/digital garden, chat, bookmark manager, workflow
  builder com IA-guia, observabilidade, etc.

## 9. Próximo bloco (depois do MVP): fluxo de pedir features ("frictionless")

Capturado aqui para não virar caixa-preta. Design detalhado merece brainstorm próprio.

O objetivo maior do usuário é **frictionless**. O `mermaid-diagram.js` já prevê tiers (o
caminho "Adicionar Feature/Bug" é leve de propósito). O problema do "frictionless" não é
*ter* tiers — é (a) ter que escolher a skill na mão e (b) o tier leve ainda ser pesado.

**Modelo proposto — 3 tiers para feature/fix em projeto existente:**

- **Tier 0 — Trivial** (typo, one-liner): zero planejamento. Cria 1 bead, dispacha 1
  agente num worktree. Sem Yegge, sem review pipeline.
- **Tier 1 — Feature pequena** (*daily driver* — o caso "mais complexo mas nem tanto"):
  planejamento leve. Lê dev session notes (`activeContext.md`, `progress.md`) + AGENTS.md → um `vc-brainstorm` enxuto → grafo pequeno
  de beads → `foolery-go` executa. Sem reviews CEO/design/devex, sem Yegge completo.
- **Tier 2 — Feature substancial / projeto novo:** pipeline completo
  (`vibe-chaos-to-concept` ou `vibe-engineering-mastery` → reviews → Yegge → beads).

**Roteador:** uma skill de entrada fina (o nó "O que vamos fazer hoje?" do diagrama) que
infere o tier (ou pergunta uma coisa) e dispacha a skill certa — o usuário nunca escolhe
skill na mão.

**Avisos:** não super-engenheirar os tiers (risco de virar frição de manutenção); o que
importa de verdade é o Tier 1 — fazê-lo realmente liso, manter 0 e 2 simples.

## 10. Referências e caminhos (para uso em sessão limpa)

| O quê | Caminho | Notas |
|-------|---------|-------|
| Repo Kernl | `/home/gabriel/repositories/kernl` | Onde a cola `[4]` e a GUI `[6]` vão morar. Não é um repositório git ainda (`git init` pendente). |
| `foolery-go` (executor) | `/home/gabriel/repositories/_cloned/foolery-go` | Backend de execução. Ler `AGENTS.md` (stack, comandos, princípios) e `specs/00-architecture.md` (blueprint autoritativo dos contratos). **`fix_plan.md` está desatualizado** — não usar como fonte de estado. |
| Skills de planejamento | `/home/gabriel/repositories/kernl/.claude/skills/` | `vibe-engineering-mastery` (caminho pesado — `vc-strategy` → reviews → `vc-writing-plans` → Yegge → `vc-convert-plan-to-beads`), `vibe-chaos-to-concept`, `vc-*`. |
| Visão geral do Kernl | `/home/gabriel/repositories/kernl/ideas.md` | Parcialmente corrigido nesta sessão (ver Seção 2 — constraint do vault descartada). |
| Fluxo completo imaginado | `/home/gabriel/repositories/kernl/mermaid-diagram.js` | Diagrama dos 5 caminhos de entrada → funil → Foolery Executor. |
| Rodar o `foolery-go` | `go run ./cmd/foolery` (dev: `air`); testes: `go test ./...` | Config: `foolery.yaml` (ver `foolery.yaml.example` no repo do `foolery-go`). Storage delega a `bd`/`kno` CLI. |

**Roteamento pós-spec:** o usuário levará este spec para `vc-strategy` dentro de
`vibe-engineering-mastery` (caminho pesado — esta é a ideia central do Kernl), e **não**
para `vc-plan`.
