# DevEx Review — DX Reference Tables

> Reference for `vc-plan-devex-review`. Consult these when scoring and benchmarking.

## DX First Principles

These are the laws. Every recommendation traces back to one of these.

1. **Zero friction at T0.** First five minutes decide everything. One click to start. Hello world without reading docs. No credit card. No demo call.
2. **Incremental steps.** Never force developers to understand the whole system before getting value from one part. Gentle ramp, not cliff.
3. **Learn by doing.** Playgrounds, sandboxes, copy-paste code that works in context. Reference docs are necessary but never sufficient.
4. **Decide for me, let me override.** Opinionated defaults are features. Escape hatches are requirements. Strong opinions, loosely held.
5. **Fight uncertainty.** Developers need: what to do next, whether it worked, how to fix it when it didn't. Every error = problem + cause + fix.
6. **Show code in context.** Hello world is a lie. Show real auth, real error handling, real deployment. Solve 100% of the problem.
7. **Speed is a feature.** Iteration speed is everything. Response times, build times, lines of code to accomplish a task, concepts to learn.
8. **Create magical moments.** What would feel like magic? Stripe's instant API response. Vercel's push-to-deploy. Find yours and make it the first thing developers experience.

## The Seven DX Characteristics

| # | Characteristic | What It Means | Gold Standard |
|---|---|---------------|---------------|
| 1 | **Usable** | Simple to install, set up, use. Intuitive APIs. Fast feedback. | Stripe: one key, one curl, money moves |
| 2 | **Credible** | Reliable, predictable, consistent. Clear deprecation. Secure. | TypeScript: gradual adoption, never breaks JS |
| 3 | **Findable** | Easy to discover AND find help within. Strong community. Good search. | React: every question answered on SO |
| 4 | **Useful** | Solves real problems. Features match actual use cases. Scales. | Tailwind: covers 95% of CSS needs |
| 5 | **Valuable** | Reduces friction measurably. Saves time. Worth the dependency. | Next.js: SSR, routing, bundling, deploy in one |
| 6 | **Accessible** | Works across roles, environments, preferences. CLI + GUI. | VS Code: works for junior to principal |
| 7 | **Desirable** | Best-in-class tech. Reasonable pricing. Community momentum. | Vercel: devs WANT to use it, not tolerate it |

## DX Scoring Rubric (0-10 calibration)

| Score | Meaning |
|-------|---------|
| 9-10 | Best-in-class. Stripe/Vercel tier. Developers rave about it. |
| 7-8 | Good. Developers can use it without frustration. Minor gaps. |
| 5-6 | Acceptable. Works but with friction. Developers tolerate it. |
| 3-4 | Poor. Developers complain. Adoption suffers. |
| 1-2 | Broken. Developers abandon after first attempt. |
| 0 | Not addressed. No thought given to this dimension. |

**The gap method:** For each score, explain what a 10 looks like for THIS product. Then fix toward 10.

## TTHW Benchmarks (Time to Hello World)

| Tier | Time | Adoption Impact |
|------|------|-----------------|
| Champion | < 2 min | 3-4x higher adoption |
| Competitive | 2-5 min | Baseline |
| Needs Work | 5-10 min | Significant drop-off |
| Red Flag | > 10 min | 50-70% abandon |

## DX Hall of Fame — Quick Reference

Instead of an external hall-of-fame file, key exemplars are embedded inline within each
review pass. Below are the gold-standard references used throughout:

**Getting Started:**
- **Stripe:** one API key, one curl, money moves. TTHW: ~30 seconds.
- **Vercel:** `git push` → deploy. TTHW: ~2 minutes.
- **Next.js:** `npx create-next-app` scaffolds a production-ready app. TTHW: ~1 minute.
- **Docker:** `docker run hello-world`. TTHW: ~2 minutes with Docker installed, ~5 minutes from zero.

**API/SDK Design:**
- **Stripe SDKs:** idiomatic per language, every parameter has a sane default. Same mental model across 7 languages.
- **Tailwind CSS:** utility classes as API — consistent naming grammar, one mental model, 95% coverage of CSS needs.
- **React Hooks:** `useState`, `useEffect` — simple primitives compose into everything. One pattern, infinite uses.

**Error Messages:**
- **Elm:** conversational, first-person ("I ran into something I wasn't expecting"), exact location, suggested fix.
- **Rust:** error codes (E0597) link to tutorials, primary + secondary labels, "help:" section with suggested fix.
- **Stripe API:** structured JSON — `{"type": "invalid_request_error", "code": "resource_missing", "message": "...", "param": "id", "doc_url": "..."}`.

**Documentation:**
- **React:** every question answered on StackOverflow, interactive playgrounds (CodeSandbox), beta docs explicitly marked.
- **MDN:** progressive disclosure, live examples, version badges, browser compatibility tables.
- **Supabase:** copy-paste snippets with YOUR project keys filled in. Real context, not placeholders.

**Upgrade Path:**
- **TypeScript:** gradual adoption — rename one file to `.ts`, it works. Never breaks JS. Strict mode is opt-in.
- **Ember:** codemods for every major version bump. `ember-cli-update` automates the migration.
- **Django:** deprecation timeline published years in advance. Warnings in development, errors in production next release.

**Dev Environment:**
- **VS Code:** language server protocol enables any language to get autocomplete/refactor. Works for junior to principal.
- **Docker:** `docker compose up` — reproducible everywhere. Single command from README clone to running app.
- **GitHub Actions:** marketplace of reusable workflows. One YAML file, CI runs.

**Community:**
- **Next.js:** conference talks, 300+ templates, Vercel marketplace, active GitHub discussions.
- **Supabase:** fully open source, active Discord (responses in minutes), community example repos.
- **Tailwind CSS:** community component galleries (Tailwind UI, Flowbite), tutorials in every framework.

**DX Measurement:**
- **Vercel Analytics:** real user metrics (Web Vitals) on every deployment, no config needed.
- **PostHog:** product analytics for developer tools — session recordings, funnel analysis, feature flags.
- **Stripe:** dashboard shows API errors by endpoint, latency percentiles, rate limit hits.
