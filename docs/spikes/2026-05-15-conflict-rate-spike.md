# Spike — Conflict Rate em Merge Topológico Sequencial

**Data:** 2026-05-15
**Disparado por:** TT1=A da eng review 2026-05-15 (`docs/reviews/vc-plan-eng-review-2026-05-15.md`)
**Objetivo:** Validar empiricamente a premissa do MergeManager de que filhos paralelos podem ser mergeados em batch no fim do épico (em vez de incremental-conforme-filho-termina).

---

## Background

O outside voice (subagent independente) da eng review desafiou a premissa central do MergeManager:

> "O spec assume que worktrees + DAG = paralelismo seguro. Mas filhos irmãos que tocam os mesmos arquivos vão gerar conflitos garantidos no merge sequencial topológico do MergeManager — e aí o 'MVP feliz' vira 'épico blocked' como caso comum, não raro. (...) Antes de codar, rodar um experimento: pegar 3 epics fechadas, simular merge topológico dos commits filho em ordem, medir taxa de conflito. Se >30%, o design precisa mudar (merge incremental por filho conforme termina, não batch no final)."

**Threshold:** >30% taxa de conflito → re-shape do design para incremental-merge.

---

## Methodology

Kernl é projeto novo sem epics históricos. **Foolery-go é o proxy ideal** — seus commits "Gap X.Y" foram literalmente dispatch real de agent swarm (multiple agents trabalhando em paralelo em sub-gaps do plano), o tipo de paralelismo que o MergeManager precisa orquestrar.

**Procedimento** (`/tmp/kernl-conflict-spike.sh`):

1. Selecionar 3 janelas contíguas de 5 commits "Gap X.Y" cada uma do histórico do foolery-go (cedo/meio/tarde do projeto).
2. Para cada janela:
   - `base` = parent commit do primeiro Gap da janela.
   - Pra cada commit Gap, criar branch `sim-<epic>-child-N` a partir de `base` e fazer `git cherry-pick` daquele commit (simula "este filho trabalhou a partir do base, isoladamente").
   - Criar `sim-<epic>-epic` a partir de `base`.
   - Sequencialmente fazer `git merge --no-ff` de cada child branch na epic branch.
3. Contar:
   - **Cherry-pick failures:** commit não consegue ser aplicado isoladamente sobre `base` (depende de estado intermediário de outro commit anterior). Sinaliza dependência implícita entre commits.
   - **Merge conflicts:** dois child branches conseguiram ser produzidos isoladamente mas conflitam ao serem mergeados juntos. **É a métrica primária pra validar a premissa.**

**Caveats:**
- Cherry-pick failures NÃO são conflito de merge — são dependências entre commits. Em epics modeladas no bd, deps explícitas no DAG serializam tais filhos (não rodam em paralelo). Em epics mal-modeladas (deps implícitas), vira problema do planejamento, não do MergeManager.
- foolery-go tem um arquivo `fix_plan.md` editado em quase todo commit. O 3-way merge resolve automaticamente na maioria dos casos (`Auto-merging fix_plan.md` aparece, sem CONFLICT). Comportamento idêntico ao kernl rodando contra arquivos comuns de coordenação tipo `AGENTS.md`, configs, schemas.

---

## Resultados

### Experimento 1 — `epic1-early` (Gap 1.x do começo do projeto)
- Base: `5b395a23` (commit anterior ao primeiro Gap)
- Children: 5 commits Gap 1.3 / 2.2 / 2.10 / 2.5 / (outro)
- **Cherry-pick failures: 4/5** (commits do início criavam arquivos que outros commits depois deletavam/modificavam — `cmd/foolery/main.go`, `fix_plan.md`, `internal/api/routes.go`)
- **Experiment SKIPPED** (apenas 1 child válido, precisa de ≥2 pro merge sequencial).

### Experimento 2 — `epic2-mid` (Gap 3.x do meio)
- Base: `f73651d6`
- Children: 5 commits Gap 3.5 / 3.8 / 3.9 / 4.5 / 5.1
- **Cherry-pick failures: 1/5** (`internal/adapter/adapter_test.go` deletado em HEAD e modificado no commit isolado)
- **Merge conflicts: 0/4 válidos**
- Auto-merges resolvidos sem conflito: `fix_plan.md` em todos (3-way merge resolveu).
- **Resultado: 4/4 merged cleanly, 0% conflict rate.**

### Experimento 3 — `epic3-late` (Gap 5.x do final)
- Base: `6580ea17`
- Children: 5 commits Gap 5.3 / 4.8 / 5.4 / 5.5 / 5.6
- **Cherry-pick failures: 0/5**
- **Merge conflicts: 0/5**
- Auto-merges: `fix_plan.md` (várias vezes), `internal/terminal/manager.go` (entre commits que tocavam o mesmo arquivo mas seções diferentes).
- **Resultado: 5/5 merged cleanly, 0% conflict rate.**

---

## Agregado

| Métrica | Valor |
|---|---|
| Epics válidos | 2 (de 3 tentados) |
| Total de merges executados | 9 |
| Total de conflitos de merge | **0** |
| **Taxa de conflito** | **0.0%** |
| Cherry-pick failures (deps implícitas) | 5/15 (33%) |

---

## Conclusão

✅ **Threshold de 30% NÃO atingido** — taxa de conflito merge texto-vs-texto = 0.0%.

✅ **Premissa do design batch-no-fim do MergeManager VALIDADA** pra o tipo de paralelismo testado.

✅ **TT1=A passa** — não é necessário pivotar pra design incremental-conforme-filho-termina.

### Caveats explícitos pra honestidade intelectual

1. **Sample size pequeno** (2 epics válidos, 9 merges). Em escala maior pode aparecer cenário não-coberto.
2. **foolery-go é proxy, não kernl.** Os domínios são similares (Go, multi-package, agent-dispatch-paralelo) mas não idênticos. Resultados em produção do kernl podem variar.
3. **33% de cherry-pick failure** refletem dependências REAIS entre commits — o que o DAG do bd protege explicitamente. Em epics bem-modeladas, filhos com deps no DAG são serializados pelo EpicExecutor, não rodam em paralelo. Em epics mal-modeladas, vira problema do **planejamento**, não do MergeManager.
4. **Arquivos coordenação (`fix_plan.md` no foolery-go, candidatos no kernl: `AGENTS.md`, `kernl.yaml`, configs, schemas)** geram `Auto-merging` em quase todo merge. 3-way merge resolveu 100% nos experimentos. Se padrão de edição mudar (vários filhos editando seções **adjacentes** do mesmo arquivo), pode aumentar conflict rate — monitorar em produção.

### Ação futura

- **Em produção**, instrumentar o MergeManager pra registrar `merge_outcome` por épico em métrica agregável. Se taxa de `merge_conflict` cruzar threshold em uso real (sugerido: 20% sustentado em 10+ épicos), reabrir esta decisão e pivotar pra incremental-merge.

---

## Reproducibility

```bash
bash /tmp/kernl-conflict-spike.sh /home/gabriel/repositories/_cloned/foolery-go
cat /tmp/kernl-spike-results.txt
```

Script ad-hoc em `/tmp/`; pode ser portado pra `scripts/spikes/conflict-rate-spike.sh` se quisermos rerun periódico.
