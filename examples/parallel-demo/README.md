# parallel-demo

Packaged epic that demonstrates Kernl's parallel bead execution.

## DAG

```
a ──→ b
  └──→ c
```

Bead `a` (Setup) runs first. After it completes, `b` (Frontend) and `c` (Backend) run concurrently.

## Usage

```bash
kernl epic run parallel-demo
```

**Warning:** this example spends real opencode tokens and wall-clock time. Each bead dispatches an opencode agent that runs the take loop against its task description.

## Expected output

- Bead `a → done` (sequential wave 1)
- Beads `b → done` and `c → done` (parallel wave 2)
- GUI URL printed on startup (e.g. `http://localhost:XXXXX`)
- Epic completes with `realized ≥ 1.5x` parallelism
