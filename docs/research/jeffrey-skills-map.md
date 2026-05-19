# Jeffrey-Skills — Mapa de Aproveitamento pro Kernl

> **Source:** 115 skills instaladas em `~/.codex/skills/` via JSM (`jsm sync`).
> **Canonical reference:** https://jeffreys-skills.md/jsm
> **Last updated:** 2026-05-15

> **Note (2026-05-18):** This skill map was compiled before the root AGENTS.md
> and orchestrator/AGENTS.md were aligned with VISION.md. References to Bubble Tea
> as a TUI option are historical — Nuxt is the settled frontend (VISION §12).

## Como ler este mapa

As skills estão organizadas em **5 buckets de relevância pro Kernl**, com prioridade decrescente:

- **A. Direct fit no orchestrator** — skills que potencialmente integram ou inspiram o **núcleo** do kernl (multi-agent dispatch, beads, agent communication).
- **B. Patterns de referência pro orchestrator** — não são integração direta mas informam decisões de design.
- **C. Tooling pro desenvolvimento do kernl** — qualidade, testing, CI, git hygiene, debugging.
- **D. Pra quando a GUI Vue/TUI aterrisar** — frontend, UI polish, deploy.
- **E. Fora do escopo do kernl** — domínios não relacionados (SaaS billing, taxes, etc).

A leitura honesta: das 115, **~25 são diretamente interessantes** (buckets A+B), **~30 servem como tooling de dev** (C), e o resto é especializado em domínios fora do kernl.

---

## A. Direct fit no orchestrator — núcleo do Kernl

Skills que poderiam ser **portadas, integradas ou usadas como referência viva** ao construir o orchestrator.

### Multi-agent / swarm coordination

- **`agent-fungibility-philosophy`** — Arquitetura de agentes fungíveis vs especializados. Recuperação de falhas, scaling de swarms. **É literalmente o design space do Kernl.** Leitura obrigatória antes de decisões de arquitetura do orchestrator.
- **`ntm`** — Multi-agent orchestration via tmux. Concorrente conceitual do kernl (abordagem diferente: tmux-based). Vale entender o que ele faz bem e o que o kernl faria melhor.
- **`vibing-with-ntm`** — Operador playbook pra babá de swarm NTM. **Catalogo dos problemas reais que aparecem em swarm**: agents stuck, rate-limited, marching orders. Cada item aqui é um caso que o kernl precisa cobrir.
- **`agent-mail`** — Inter-agent communication via Agent Mail. **Conecta com o item do BACKLOG sobre MCP agent mail** — entender essa skill antes de avaliar a integração.
- **`code-review-gemini-swarm-with-ntm`** — Padrão de code review com múltiplos agents. Referência pra pipeline review futuro no kernl.
- **`modes-of-reasoning-project-analysis`** — Multi-perspective via NTM swarm. Padrão "vários agents com perspectivas diferentes opinam".
- **`dueling-idea-wizards`** — Geração de ideias adversarial entre agents. Referência pra workflow de brainstorm orquestrado.
- **`multi-model-triangulation`** — Cross-model evaluation. Padrão pra decisões que envolvem múltiplos modelos.
- **`multi-pass-bug-hunting`** — Debug iterativo via agents. Workflow de debugging orquestrado.
- **`repeatedly-apply-skill`** — Loop "aplica skill N vezes". Padrão de iterative refinement.

### Beads integration

- **`beads-br`** — Issue tracker beads_rust (jeffrey's Rust port). Kernl usa `bd` (gastownhall/beads) — mas **a versão rust pode ser relevante** se valer migração futura. Confere com TODOS.md sobre "bd 1.0.4".
- **`beads-bv`** — Graph-aware triage com `bv` + `br`. Padrão de "priorizar por bottleneck no DAG" — diretamente aplicável ao scheduler do kernl.
- **`beads-workflow`** — Markdown plan → beads (você já usa).
- **`beads-compliance-and-completion-verification`** — Audit "issues fechadas foram realmente implementadas?". **Útil pro próprio kernl** dado o drift bd↔git que apontei antes.
- **`bd-to-br-migration`** — Migração bd → br. Backlog item se migrar for decidido.

### Agent safety / runtime

- **`slb`** (Simultaneous Launch Button) — Two-person rule pra comandos destrutivos. Padrão pra **multi-agent safety no kernl** (ex: gate humano antes de `git push --force`).
- **`dcg`** — Bloqueio de destructive commands. Mesma área que `slb`.
- **`world-class-doctor-mode-for-cli-tools`** — `doctor` subcommand robusto. Kernl já tem `doctor`; vale comparar com o padrão "ouro" desta skill.
- **`agent-ergonomics-and-intuitiveness-maximization-for-cli-tools`** — Como deixar uma CLI agent-friendly. **Aplicação direta no `kernl` CLI** — kernl é justamente "CLI consumida por agents + humanos".

### Memory / state

- **`cass-memory`** — CASS Memory System (`cm`) — memória procedural pra agents. **Concorrente conceitual do plano "claude-memory-bank refactor" no BACKLOG** — vale entender antes de partir pro refactor.
- **`cass`** — Mine past agent sessions for prompts, decisions, patterns. Padrão "aprender com o histórico" — relevante pra observabilidade do kernl.
- **`caam`** — Sub-100ms account switching entre Claude Max / GPT Pro / Gemini Ultra. **Já implementa parte de "agent fungibility"** — vale ver se o kernl pode delegar essa parte ou se incorpora.

### MCP

- **`mcp-server-design`** — Design de MCP servers agent-friendly. Relevante se o kernl expuser MCP (e do BACKLOG: MCP agent mail).

---

## B. Patterns de referência pro orchestrator

Não são integração direta, mas informam decisões.

- **`planning-workflow`** — Metodologia de plan markdown (você usa).
- **`vibe-engineering-mastery`** — Pipeline strategy → reviews → planning. **Próximo passo natural depois do brainstorm de visão.**
- **`operationalizing-expertise`** — Destilar expertise em rules/operators/validators executáveis. Relevante pra transformar os specs do kernl em "executable rules" do orchestrator.
- **`idea-wizard`** — Gerar ideias de melhoria + virar beads. Padrão "what should we build next" — útil pra inbox de melhorias do próprio kernl.

---

## C. Tooling pro desenvolvimento do kernl

Skills pra usar **enquanto construindo o kernl**.

### Testing (kernl tem 962 unit tests + integration)
- **`testing-fuzzing`** — Fuzz harnesses (Go é suportado). Aplicável a `workflow/`, `merge/`, `sweep/`.
- **`testing-metamorphic`** — Metamorphic testing pra sistemas com oracle problem (orchestrator! "saída correta é desconhecida mas relações input-output são previsíveis").
- **`testing-conformance-harnesses`** — Verificar implementações contra specs. **Diretamente aplicável**: `orchestrator/specs/` define behavior; harness valida.
- **`testing-golden-artifacts`** — Golden file testing. Útil pra fixtures de bead/epic.
- **`testing-real-service-e2e-no-mocks`** — E2E sem mocks. Alinha com Lane H do plano atual.

### Concurrency / performance
- **`deadlock-finder-and-fixer`** — Bugs de concorrência (locks, races, await-holding-lock). **Crítico**: kernl é goroutine-per-session, single-flight no MergeManager.
- **`gdb-for-debugging`** — Debug de processo travado / hang. Pode salvar muita hora.
- **`profiling-software-performance`** — Profile, flamegraph, hotspot.
- **`extreme-software-optimization`** — Otimização baseada em profile.

### Code quality / review
- **`ubs`** (Ultimate Bug Scanner) — Code review / bug scan. Pre-commit ou pre-PR.
- **`codebase-audit`** — Audit parameterizado por domínio (security, UX, perf, etc).
- **`codebase-report`** — Producir architecture docs reusáveis. **Útil pra próximo onboarding** (relaciona com BACKLOG: onboarding/CONTRIBUTING).
- **`codebase-archaeology`** — Exploration.
- **`codebase-pattern-extraction`** — Extract patterns.
- **`reality-check-for-project`** — **Assess project vs README/plan vision. Literalmente o que você pediu hoje** ("falta clareza do sistema final").
- **`simplify-and-refactor-code-isomorphically`** — Shrink/unify sem mudar comportamento.
- **`de-slopify`** — Limpar slop de código gerado por LLM.
- **`mock-code-finder`** — Find stubs/mocks/TODOs. **Aplicar antes de declarar MVP done.**

### Git / repo hygiene
- **`git-worktree-branch-rationalization`** — Kernl usa worktrees! Inevitável precisar.
- **`git-stash-janitor`** — Stash archaeology.
- **`git-repo-janitor`** — Limpar arquivos junk committados (skills/swarms deixam lixo).
- **`path-rationalization`** — Rationalize paths.

### Release / CI
- **`gh-actions`** — CI/CD pra Go (kernl é Go).
- **`release-preparations`** — Preparar releases.
- **`changelog-md-workmanship`** — Changelog.
- **`gh-cli`**, **`gh-triage-ru`** — GitHub triage.
- **`library-updater`** — Bump deps (go.mod).
- **`readme-writing`** — README.
- **`documentation-website-for-software-project`** — Nextra docs site (eventualmente).
- **`installer-workmanship`** — curl|bash installer (eventualmente — kernl não tem ainda).

### Misc dev
- **`research-software`** — Pesquisar tools via source/web. Útil quando avaliar libs.
- **`rg-optimized`** — ripgrep otimizado.
- **`rch`** — Remote compilation offload.
- **`cc-hooks`** — Configurar hooks do Claude Code (você usa muito).

---

## D. Pra quando a GUI Vue/TUI aterrissar

A STRATEGY menciona Vue 3 + Nuxt (VISION §12). Backlog item: "GUI inicial com Vue".

- **`tui-glamorous`** — Bubble Tea (Go TUI) — direct fit se a TUI vier.
- **`tui-inspector`** — TUI debugging.
- **`ui-polish`** — Iterative UI polish (post-functional).
- **`frankentui`** — UI components.
- **`frankensuite-website-development`** — Frankensuite quality (referência alto-padrão).
- **`og-share-images`**, **`gh-og-share-images`** — Social previews.
- **`interactive-visualization-creator`** — Visualizações.
- **`ux-audit`** — UX audit.
- **`tanstack`** — TanStack (mais React, mas a filosofia "adopt strategically" se aplica).
- **`vercel`**, **`wrangler`** — Deploy (se virar SaaS).

---

## E. Fora do escopo do kernl

Domínios não relacionados (mantenho a lista pra você saber que estão lá e ignorar):

SaaS comercial:
- `saas-billing-patterns-for-stripe-and-paypal`, `stripe-checkout`, `saas-customer-analytics`, `security-audit-for-saas`, `seo-for-saas-businesses`, `admin-page-for-nextjs-sites`, `user-support-ticketing-system-for-saas`, `user-support-triage-for-saas-and-open-source-projects`, `ab-testing`, `saas-cli-auth-flow`, `ga4`, `supabase`

Migrações específicas:
- `slack-migration-to-mattermost-phase-1/2/3`, `slack-migration-to-mattermost-*`

Domínios pessoais:
- `tax-return-preparation-and-advice-generic`, `wills-and-estate-planning-skill`, `video-obs-youtube-music`, `xf` (X/Twitter mining)

Outras tools especializadas que não são pro kernl:
- `brenner` (research bot pessoal), `pi-agent-rust` (outro projeto Rust), `csctf` (CTF / share-link archive), `asupersync-mega-skill` (Rust async runtime), `lean-formal-feedback-loop` (Lean prover), `gcloud`, `ssh`, `ghostty`, `wezterm`, `cursor`, `dsr`, `automating-your-automations`, `pi-agent-rust`, `ru-multi-repo-workflow`, `keybindings-help`, `giil`, `frankensearch-integration-for-rust-projects`, `browser-extension-automation`, `system-performance-remediation`, `e2e-testing-for-webapps` (Playwright/Next.js, fora do Go), `rust-cli-with-sqlite`, `rust-crates-publishing`, `rust-undefined-behavior-exorcist`, `rust-unsafe-code-exorcist`, `pi-agent-rust`

---

## Recomendações de próximos passos

Em ordem de leverage:

1. **Ler `agent-fungibility-philosophy`** — informa decisões arquiteturais do orchestrator. ~30min.
2. **Ler `agent-ergonomics-and-intuitiveness-maximization-for-cli-tools`** — kernl é CLI consumida por agents. Aplicar antes da v1 da CLI estabilizar.
3. **Rodar `reality-check-for-project`** no kernl — endereça diretamente seu "falta clareza do sistema final".
4. **Avaliar `agent-mail` + `cass-memory`** antes de decidir BACKLOG items "MCP agent mail" e "claude-memory-bank refactor" — pode ser que essas skills já resolvem ou conflitam.
5. **Antes do brainstorm de visão**: revisar `vibing-with-ntm` — é o catálogo concreto dos problemas que aparecem em swarm de agents real, e ajuda a ancorar a discussão de "state of the art" em casos reais ao invés de abstratos.
6. **Quando aterrissar o GUI**: revisar bucket D inteiro.
7. **Quando o kernl tiver users externos**: revisar `installer-workmanship` + `documentation-website-for-software-project` + `readme-writing`.

## Notas

- Esse mapa é melhor lido em conjunto com `BACKLOG.md` (várias skills se mapeiam direto em itens lá) e `TODOS.md` (algumas resolvem TODOs).
- Skills evoluem (são gerenciadas por JSM). Re-validar este mapa ao rodar `jsm sync` ocasionalmente.
- A análise é baseada nas descrições das skills no system prompt + spot-checks nos `SKILL.md`. Não é exaustiva — algumas skills podem ter capabilities além da descrição.
