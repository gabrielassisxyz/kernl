# Lint Debt

This file tracks the temporary lint backlog surfaced by `golangci-lint`.

## Current Baseline

Run from the repository root:

```bash
golangci-lint run ./...
```

As of 2026-06-16, the advisory baseline is:

- `errcheck`: 50
- `unused`: 49
- `staticcheck`: 33
- `ineffassign`: 2
- `gofmt`: 1

Total: 135 issues.

## Cleanup Order

1. Fix `gofmt` and `ineffassign`; these are mechanical and low-risk.
2. Remove genuinely dead `unused` code; keep test fakes only when they still
   encode a useful interface contract.
3. Fix `staticcheck` findings that are behavior-preserving.
4. Fix `errcheck` in production code before test cleanup helpers.
5. Remove `--issues-exit-code=0` from CI and flip the lint job from advisory to
   required once `golangci-lint run ./...` is clean.

## Gate Policy

Until this backlog is burned down, CI runs lint in advisory mode so findings stay
visible without blocking unrelated work. New code should not add new lint debt.
