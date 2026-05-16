# Backlog

Captura rápida — ideias que aparecem ao longo do dia. Low-ceremony: 1-3 linhas, sem rationale obrigatório.

Quando amadurece (decisão de fazer + contexto suficiente): migra pra `TODOS.md` ou vira issue no `bd`.

---

## Orquestração / produto

- **Aba do orquestrador estilo "mayor"** (ref: https://github.com/gastownhall/gastown), mais intuitiva. Recebe 1+ tasks; a skill / `AGENTS.md` do orquestrador sugere o nível de aprofundamento necessário pra cada uma (brainstorm, review(s), planning, etc).
- **Refatorar (modernizar) o claude-memory-bank e integrar no repo** (https://github.com/russbeye/claude-memory-bank). Reaproveitar o que já está salvo em `~/.claude/projects`.
- **MCP agent mail — como integraria?** https://github.com/Dicklesworthstone/mcp_agent_mail — avaliar encaixe com beads + orchestrator.

## Pesquisa / referência

- **Mapear jeffrey-skills** (https://jeffreys-skills.md/jsm, instaladas em `~/.codex/skills`). Visão geral + análise de quais aproveitar/usar de referência: (a) no orchestrator do kernl, (b) no desenvolvimento do projeto kernl como um todo.
- **Overprompting como referência ao escrever prompts entre agents do orchestrator**: https://www.jeffreyemanuel.com/writing/overprompting.

## Tooling de planejamento

- **`vc-convert-plan-to-beads` — plan em formato script-readable?** As duas execuções rodaram via script de extração. Plans estão em ~32k tokens — chance da LLM se perder/esquecer é alta. Talvez o plan já deva ser escrito em formato estruturado pra conversão determinística (sem LLM no meio).
