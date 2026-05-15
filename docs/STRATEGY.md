---
name: Kernl — Núcleo de Orquestração
last_updated: 2026-05-14
---

# Kernl — Núcleo de Orquestração — Strategy

## Target problem

Como desenvolvedor solo, você consegue idealizar mais do que consegue executar. O
gargalo não é ter ideias nem saber planejá-las — é que executá-las exige gerenciar
múltiplas sessões/agentes em paralelo, cada uma num step diferente, e a banda humana pra
trocar de contexto entre elas não escala. O trabalho que só você faz bem (idealizar,
direcionar, revisar) fica refém do trabalho de babá que você faz mal.

## Our approach

O humano só toca nos pontos de julgamento; o resto é um grafo de beads que os agentes
executam em paralelo sem supervisão contínua. A aposta é no grafo de dependências com
paralelismo real + gates de julgamento humano discretos — não num assistente pilotado
turn-a-turn, nem em dispatch de uma task por vez — para que a banda humana deixe de ser
o gargalo da execução.

## Who it's for

**Primary:** Desenvolvedor solo construindo as próprias ferramentas — você. Está
contratando o Kernl pra manter idealização e execução alinhadas sem virar babá de
agentes: transformar uma ideia em épico executado em paralelo, aparecendo só nos gates
(direcionar planejamento, aprovar, revisar).

## Key metrics

- **Intervenções fora de gate por épico** — quantas vezes o humano entrou na execução
  fora dos gates planejados. Leading; deve cair. Mede o gargalo direto.
- **Paralelismo realizado** — média de sessões-agente simultâneas ÷ máximo que o grafo
  permitiria. Mede se o paralelismo real acontece ou se na prática vira sequencial.
- **Épicos concluídos sem resgate manual** — % de épicos que terminam sem o humano cair
  em modo babá. Lagging; o par de limiar da primeira métrica.
- **Ideias que viraram épico executado / mês** — o gargalo realmente se moveu. Lagging;
  regride se a pipeline emperrar.

_Nenhuma é medida hoje. Ponto de partida provável: o que o `foolery-go` já registra de
sessões e estado de beads._

## Tracks

### Motor de execução paralela

O `foolery-go` (adaptado e finalmente rodando), a cola planejamento↔execução e o
isolamento por worktree. Faz o grafo de beads rodar em paralelo de verdade.

_Why it serves the approach:_ é a metade "grafo executado em paralelo" — sem isso não há
o que tirar do humano.

### Observabilidade da orquestração

O painel/GUI consumindo SSE: quem faz o quê, estado dos beads, flag de erro.

_Why it serves the approach:_ o humano só consegue ficar fora da execução se enxergar o
estado sem entrar nela — sem observabilidade, "supervisão não-contínua" vira voo cego.

### Loop de gates de julgamento

Os pontos discretos onde o humano entra: aprovação de plano e o loop de
integração/review/merge.

_Why it serves the approach:_ é a outra metade — define onde o humano toca, pra que não
toque em mais nada.

## Milestones

- **2026-05-16 (alvo)** — MVP do orchestrator: pronto quando o Passo A (1 bead real
  executado) e o Passo B (1 épico real em paralelo) passam e a GUI mínima mostra o épico
  rodando. A data é alvo, não prazo rígido — a condição de conclusão é o critério real.

## Marketing

**One-liner:** Orquestração multi-agente em que o humano só toca nos pontos de
julgamento — o resto é um grafo executado em paralelo.

**Key message:** Open source desde o princípio, projetado em blocos — cada um usa o
bloco que quiser e configura do seu jeito. Nasce pra resolver uma dor própria, mas
voltado pra estar disponível pra todo mundo. SaaS é uma possibilidade futura, não o
ponto de partida.
