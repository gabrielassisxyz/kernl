#!/usr/bin/env python3
"""
swarm_parallel.py — 3-worker parallel executor + merge orchestrator for kernl beads

Reads ready beads from `bd`, dispatches up to 3 in parallel (deepseek-v4-pro-max),
then calls a merge orchestrator (kimi-k2.6) to reconcile branches and ff-merge to master.

This replaces swarm_bootstrap.py once the merge-orchestrator flow is proven stable.
"""
from __future__ import annotations

import argparse
import dataclasses
import json
import os
import subprocess
import sys
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional, TextIO

# ─── paths & config ──────────────────────────────────────────────
KERNL_REPO       = Path("/home/gabriel/repositories/kernl")
WORKTREE_ROOT    = Path("/home/gabriel/repositories/kernl-swarm-worktrees")
SWARM_DIR        = KERNL_REPO / "scripts" / "swarm"
LOGS_DIR         = SWARM_DIR / "logs"
OPENCODE_CONFIG  = SWARM_DIR / "opencode-config.json"
AGENTS_MD        = KERNL_REPO / "AGENTS.md"

MODEL_PRIMARY        = "litellm/deepseek-v4-pro-max"
MODEL_ORCHESTRATOR   = "litellm/kimi-k2.6"
MODEL_CLAUDE         = "claude-sonnet-4-6"
ATTEMPTS_PER_MODEL   = 5
PRE_MERGE_FIX_RETRIES = 2   # re-spawn worker with go-test output if exit-0 but tests fail
RETRY_SLEEP_SECONDS  = 30
PER_ATTEMPT_TIMEOUT  = 90 * 60
PER_BEAD_TIMEOUT     = 4 * 60 * 60
MASTER_BRANCH        = "master"
MAX_WORKERS          = 3

# ─── ANSI ────────────────────────────────────────────────────────
RESET="\033[0m"; BOLD="\033[1m"; GREEN="\033[92m"
YELLOW="\033[93m"; RED="\033[91m"; BLUE="\033[94m"; DIM="\033[2m"
def _ts(): return datetime.now().strftime("%H:%M:%S")
def info(m): print(f"{DIM}{_ts()}{RESET} {BLUE}[swarm]{RESET} {m}", flush=True)
def ok(m):   print(f"{DIM}{_ts()}{RESET} {GREEN}[swarm]{RESET} {m}", flush=True)
def warn(m): print(f"{DIM}{_ts()}{RESET} {YELLOW}[swarm]{RESET} {m}", flush=True)
def err(m):  print(f"{DIM}{_ts()}{RESET} {RED}[swarm]{RESET} {m}", file=sys.stderr, flush=True)

# ─── data ──────────────────────────────────────────────────────
@dataclasses.dataclass
class Bead:
    id: str
    title: str
    issue_type: str
    priority: int
    description: str
    metadata_files: dict

    @property
    def branch(self) -> str: return f"kernl-swarm/{self.id}"
    @property
    def worktree(self) -> Path: return WORKTREE_ROOT / self.id

# ─── bd interactions ────────────────────────────────────────────
def _bd(*args, capture=True) -> subprocess.CompletedProcess:
    return subprocess.run(["bd", *args], cwd=KERNL_REPO,
                          capture_output=capture, text=True, check=False)

def fetch_ready_beads() -> list[Bead]:
    """
    Return open tasks/chores whose only remaining blockers are parent-child
    (epic) links — never real blocking dependencies. The bd CLI treats any
    open dependency as a blocker, so we bypass `bd ready` and evaluate ourselves.
    """
    r = _bd("list", "--status", "open", "--json")
    if r.returncode != 0:
        raise RuntimeError(f"bd list failed: {r.stderr}")
    issues = json.loads(r.stdout)

    # Build lookup of open issue IDs
    open_ids = {i["id"] for i in issues}

    out: list[Bead] = []
    for raw in issues:
        if raw.get("issue_type") == "epic":
            continue
        deps = raw.get("dependencies", [])
        # A bead is ready if every dependency of type 'blocks' is already closed
        blocked = False
        for dep in deps:
            if dep.get("type") == "blocks" and dep.get("depends_on_id") in open_ids:
                blocked = True
                break
        if blocked:
            continue
        bead = _show_bead(raw["id"])
        if bead is not None:
            out.append(bead)
    out.sort(key=lambda b: (b.priority, b.id))
    return out

def _show_bead(bead_id: str) -> Optional[Bead]:
    r = _bd("show", bead_id, "--json")
    if r.returncode != 0:
        return None
    data = json.loads(r.stdout)
    if isinstance(data, list):
        data = data[0]
    md = data.get("metadata") or {}
    files_raw = md.get("files")
    files = {}
    if isinstance(files_raw, str):
        try:
            files = json.loads(files_raw)
        except json.JSONDecodeError:
            files = {}
    elif isinstance(files_raw, dict):
        files = files_raw
    return Bead(
        id=data["id"],
        title=data.get("title", ""),
        issue_type=data.get("issue_type", "task"),
        priority=int(data.get("priority", 2)),
        description=data.get("description", "") or "",
        metadata_files=files,
    )

def close_bead(bead_id: str, reason: str) -> bool:
    r = _bd("close", bead_id, "--reason", reason)
    if r.returncode != 0:
        err(f"bd close {bead_id} failed: {r.stderr.strip()}")
        return False
    return True

# ─── git helpers ─────────────────────────────────────────────────
def _git(*args, cwd: Path = KERNL_REPO, check=True) -> subprocess.CompletedProcess:
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, check=check)

def current_master_head() -> str:
    return _git("rev-parse", MASTER_BRANCH).stdout.strip()

def ensure_on_master_clean() -> None:
    cur = _git("branch", "--show-current").stdout.strip()
    if cur != MASTER_BRANCH:
        err(f"current branch is {cur!r}, expected {MASTER_BRANCH!r}.")
        sys.exit(2)
    r = _git("status", "--porcelain")
    if r.stdout.strip():
        err(f"{MASTER_BRANCH} has uncommitted changes — clean working tree required.")
        sys.exit(2)

def create_worktree(bead: Bead, base: str) -> None:
    if bead.worktree.exists():
        warn(f"  worktree {bead.worktree} already exists — pruning")
        remove_worktree(bead)
    _git("worktree", "add", "-b", bead.branch, str(bead.worktree), base)

def remove_worktree(bead: Bead) -> None:
    if bead.worktree.exists():
        _git("worktree", "remove", "-f", str(bead.worktree), check=False)
    _git("worktree", "prune", check=False)
    _git("branch", "-D", bead.branch, check=False)

def ff_merge_branch(branch: str) -> tuple[bool, str]:
    """Fast-forward merge a single branch into master. Returns (ok, stderr)."""
    r = _git("merge", "--ff-only", branch, check=False)
    if r.returncode == 0:
        ok(f"  fast-forwarded {branch}")
        return True, ""
    return False, r.stderr

def merge_branch_non_ff(branch: str) -> tuple[bool, str]:
    """Try a regular merge. Returns (ok, stderr)."""
    r = _git("merge", "--no-ff", "-m", f"merge {branch}", branch, check=False)
    if r.returncode == 0:
        return True, ""
    return False, r.stderr

def reset_merge() -> None:
    """Abort a failed merge."""
    _git("merge", "--abort", check=False)

# ─── worker (single bead, no merge to master) ────────────────────
class WorkerResult:
    def __init__(self, bead: Bead):
        self.bead = bead
        self.success = False
        self.commit_sha: Optional[str] = None
        self.error: Optional[str] = None

def run_worker(bead: Bead, dry_run: bool, use_claude: bool = False, resume: bool = False, tailer_lock: Optional[threading.Lock] = None) -> WorkerResult:
    result = WorkerResult(bead)
    if dry_run:
        info(f"[DRY-RUN] would dispatch {bead.id} {bead.title!r}")
        result.success = True
        return result

    base = current_master_head()
    if resume and bead.worktree.exists():
        info(f"[{bead.id}] --resume: reusing existing worktree {bead.worktree}")
    else:
        create_worktree(bead, base)
    prompt = build_prompt(bead)

    bead_start = time.monotonic()
    session_id: Optional[str] = None
    primary = MODEL_CLAUDE if use_claude else MODEL_PRIMARY
    for stage, model in (("primary", primary), ("fallback", MODEL_ORCHESTRATOR)):
        for attempt_num in range(1, ATTEMPTS_PER_MODEL + 1):
            elapsed = time.monotonic() - bead_start
            if elapsed >= PER_BEAD_TIMEOUT:
                result.error = f"per-bead budget exhausted ({elapsed/60:.0f}min)"
                return result
            info(f"[{bead.id}] attempt {attempt_num}/{ATTEMPTS_PER_MODEL} model={model}")
            att = Attempt(bead, model, session_id, stdout_sink=sys.stdout, tailer_lock=tailer_lock)
            att.spawn(prompt)
            rc = att.wait_with_timeout()
            # session ID may have been captured by the tailer
            if _tailer_mod is not None and hasattr(att._tailer, 'session_id'):
                sid = att._tailer.session_id
                if sid:
                    session_id = sid
            elif att.captured_session_id:
                session_id = att.captured_session_id
            verdict = att.classify()
            if verdict == "success":
                # verify tests; if red, re-spawn the same session with the failure
                # output injected. The worker often exits rc=0 with broken code
                # when it runs out of context mid-task; a focused follow-up with
                # the exact compile/test error usually fixes it cheaply.
                fix_attempt = 0
                while True:
                    gomod_dir = bead.worktree / "orchestrator"
                    r = subprocess.run(["go", "test", "./..."], cwd=gomod_dir,
                                       capture_output=True, text=True)
                    if r.returncode == 0:
                        head = _git("-C", str(bead.worktree), "rev-parse", "HEAD",
                                    check=False).stdout.strip()
                        result.commit_sha = head
                        result.success = True
                        return result
                    # tests failed
                    (LOGS_DIR / f"{bead.id}.gotest.failed.txt").write_text(
                        r.stdout + "\n--- stderr ---\n" + r.stderr)
                    if fix_attempt >= PRE_MERGE_FIX_RETRIES or session_id is None:
                        result.error = "go test failed"
                        return result
                    fix_attempt += 1
                    warn(f"[{bead.id}] go test failed — pre-merge fix attempt "
                         f"{fix_attempt}/{PRE_MERGE_FIX_RETRIES} (resuming session)")
                    fix_prompt = build_fix_prompt(bead, r.stdout, r.stderr)
                    fix = Attempt(bead, model, session_id)
                    fix.spawn(fix_prompt)
                    fix.wait_with_timeout()
                    if fix.captured_session_id:
                        session_id = fix.captured_session_id
                    # loop back and re-run go test regardless of fix's exit code
            if verdict == "session-lost":
                session_id = None
            if attempt_num < ATTEMPTS_PER_MODEL:
                time.sleep(RETRY_SLEEP_SECONDS)
    result.error = "both models exhausted"
    return result

# ─── Tailer import (human-readable live logs while writing NDJSON) ──
import importlib.util
_tailer_mod = None
_tailer_path = SWARM_DIR / "swarm_tail.py"
if _tailer_path.exists():
    _spec = importlib.util.spec_from_file_location("swarm_tail", str(_tailer_path))
    if _spec is not None and _spec.loader is not None:
        _tailer_mod = importlib.util.module_from_spec(_spec)
        _spec.loader.exec_module(_tailer_mod)

# ─── Attempt class ───────────────────────────────────────────────
class Attempt:
    def __init__(self, bead: Bead, model: str, session_id: Optional[str],
                 stdout_sink: Optional[TextIO] = None,
                 tailer_lock: Optional[threading.Lock] = None):
        self.bead = bead
        self.model = model
        self.session_id = session_id
        self._is_claude = model == MODEL_CLAUDE
        self.captured_session_id: Optional[str] = None
        self.stderr_buf: list[str] = []
        self.exit_code: Optional[int] = None
        self.proc: Optional[subprocess.Popen] = None
        self.stdout_sink = stdout_sink
        self.tailer_lock = tailer_lock
        self._err_reader: Optional[threading.Thread] = None
        self._tailer: Optional[object] = None  # swarm_tail.Tailer if available

    def cmd(self, prompt: str) -> list[str]:
        if self._is_claude:
            c = ["claude", "-p",
                 "--model", self.model,
                 "--cwd", str(self.bead.worktree)]
            if self.session_id:
                c += ["--resume", self.session_id]
            c += [prompt]
            return c
        c = ["opencode", "run",
             "--format", "json",
             "--dir", str(self.bead.worktree),
             "-m", self.model,
             "-f", str(AGENTS_MD),
             "--title", f"kernl-swarm:{self.bead.id}"]
        if self.session_id:
            c += ["-s", self.session_id]
        c += [prompt]
        return c

    def spawn(self, prompt: str) -> None:
        env = os.environ.copy()
        env["OPENCODE_CONFIG"] = str(OPENCODE_CONFIG)
        env["GOFLAGS"] = "-count=1"
        cmd = self.cmd(prompt)
        extra: dict = {"cwd": str(self.bead.worktree)} if self._is_claude else {}
        self.proc = subprocess.Popen(
            cmd, env=env,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, bufsize=1,
            **extra,
        )

        # --- stdout: plain text for claude, NDJSON for opencode ---
        log_path = LOGS_DIR / f"{self.bead.id}-{datetime.now().strftime('%Y%m%d-%H%M%S')}.jsonl"
        if self._is_claude:
            txt_path = log_path.with_suffix(".claude.log")
            sink = self.stdout_sink or sys.stdout
            lock = self.tailer_lock
            def _claude_drain():
                assert self.proc is not None and self.proc.stdout is not None
                with open(txt_path, "a", encoding="utf-8") as fh:
                    for line in self.proc.stdout:
                        fh.write(line); fh.flush()
                        if lock:
                            with lock:
                                sink.write(line); sink.flush()
                        else:
                            sink.write(line); sink.flush()
            t = threading.Thread(target=_claude_drain, daemon=True)
            t.start()
            self._tailer = t
        elif _tailer_mod is not None:
            render_sink = self.stdout_sink
            if self.tailer_lock is not None:
                class LockingSink:
                    def __init__(self, lock: threading.Lock, real: TextIO):
                        self._lock = lock
                        self._real = real
                    def write(self, data: str) -> None:
                        with self._lock:
                            self._real.write(data)
                    def flush(self) -> None:
                        with self._lock:
                            self._real.flush()
                render_sink = LockingSink(self.tailer_lock, self.stdout_sink or sys.stdout)

            self._tailer = _tailer_mod.Tailer(
                bead_id=self.bead.id,
                log_path=log_path,
                render_sink=render_sink,
                show_tools=True,
                show_system=False,
            )
            self._tailer.start(self.proc.stdout)
        else:
            # Fallback: just write JSONL if tailer module is missing
            def _jsonl_drain():
                assert self.proc is not None and self.proc.stdout is not None
                with open(log_path, "a", encoding="utf-8") as fh:
                    for line in self.proc.stdout:
                        fh.write(line); fh.flush()
                        if self.captured_session_id is None:
                            line_s = line.strip()
                            if line_s.startswith("{") and "sessionID" in line_s:
                                try:
                                    ev = json.loads(line_s)
                                    sid = ev.get("sessionID")
                                    if isinstance(sid, str) and sid.startswith("ses_"):
                                        self.captured_session_id = sid
                                except json.JSONDecodeError:
                                    pass
            t = threading.Thread(target=_jsonl_drain, daemon=True)
            t.start()
            self._tailer = t  # type: ignore[assignment]

        # --- stderr: keep for error classification ---
        self._err_reader = threading.Thread(target=self._drain_stderr, daemon=True)
        self._err_reader.start()

    def _drain_stderr(self) -> None:
        assert self.proc is not None and self.proc.stderr is not None
        with open(LOGS_DIR / f"{self.bead.id}-{datetime.now().strftime('%Y%m%d-%H%M%S')}.stderr.log", "a", encoding="utf-8") as fh:
            for line in self.proc.stderr:
                fh.write(line); fh.flush()
                s = line.strip()
                if s:
                    self.stderr_buf.append(s)
                    if len(self.stderr_buf) > 50:
                        self.stderr_buf.pop(0)

    def wait_with_timeout(self) -> int:
        assert self.proc is not None
        try:
            self.exit_code = self.proc.wait(timeout=PER_ATTEMPT_TIMEOUT)
        except subprocess.TimeoutExpired:
            warn(f"[{self.bead.id}] attempt timed out — killing")
            self.proc.kill()
            self.exit_code = self.proc.wait()
            self.stderr_buf.append("ATTEMPT TIMED OUT")
        if _tailer_mod is not None and hasattr(self._tailer, 'stop'):
            self._tailer.stop(timeout=5)
        elif isinstance(self._tailer, threading.Thread):
            self._tailer.join(timeout=5)
        if self._err_reader:
            self._err_reader.join(timeout=5)
        return self.exit_code or 0

    def classify(self) -> str:
        if self.exit_code == 0:
            return "success"
        blob = " ".join(self.stderr_buf).lower()
        if any(m in blob for m in ("session not found",)):
            return "session-lost"
        if any(m in blob for m in (
            "rate limit", "ratelimit", "429", "502", "503", "504",
            "timeout", "timed out", "connection error", "connection reset",
            "litellm", "provider returned error", "badrequesterror",
            "overloaded", "service unavailable", "internal server error",
        )):
            return "transient"
        return "transient"

# ─── prompt builder ──────────────────────────────────────────────
def build_prompt(bead: Bead) -> str:
    files_json = json.dumps(bead.metadata_files, indent=2) if bead.metadata_files else "(none listed)"
    return f"""# Bead {bead.id} — {bead.title}

You are an autonomous engineer executing ONE bead from the kernl
orchestrator-core implementation plan. The cwd you are running in is a git
worktree dedicated to this bead. Follow the Steps below exactly and stop
when the bead is complete.

## Steps (verbatim from the approved plan)

{bead.description}

## Operating rules

1. Edit ONLY files within this worktree, primarily those listed in `Files`
   below. Do not touch unrelated packages or files in other beads' scope.
2. Follow AGENTS.md style: files < 500 lines, funcs 4–40 lines, fail-loud
   marker `KERNL DISPATCH FAILURE: <problem> — <cause> — Fix: <action>`.
3. Tests must be hermetic (`*_test.go`) using fakes/stubs. No real network,
   no real disk outside of t.TempDir().
4. The Go module lives at `orchestrator/go.mod` — NOT at the worktree root.
   Before declaring done, you MUST run (in this exact order):
   ```bash
   cd {KERNL_REPO / "orchestrator"} && go vet ./... && go test ./...
   ```
   This runs the ENTIRE module's tests so you catch regressions in other
   packages, not just the package you touched. **If ANY test fails, you are NOT
   done — go back, fix the failing tests, and re-run.** Do NOT leave failing
   tests.
5. **CHECK GATE — you MUST run both before AND after the `git commit`.**
   After `go test ./...` passes, only then:
   ```bash
   cd {KERNL_REPO} && git add -A && git commit -m "<conventional message>"
   ```
   Use one commit per bead unless the plan explicitly calls for more.
6. DO NOT push. DO NOT switch branches. DO NOT touch `{MASTER_BRANCH}`.
   DO NOT run `bd close` — the orchestrator does that.
7. If you cannot proceed because of a missing dependency, fail loud with a
   descriptive error and stop. Do not invent stubs.

## Bead metadata
- ID: {bead.id}
- Priority: P{bead.priority}
- Files declared by the plan:
```json
{files_json}
```
"""

def build_fix_prompt(bead: Bead, stdout: str, stderr: str) -> str:
    """Follow-up prompt: same session, told that tests are red and what failed.

    Truncate to ~6KB to keep cache hits and respect the worker's remaining
    context budget — the full log is on disk if the agent really needs it.
    """
    out = (stdout or "").strip()
    err = (stderr or "").strip()
    if len(out) > 4000:
        out = out[:2000] + "\n...[truncated]...\n" + out[-1800:]
    if len(err) > 2000:
        err = err[:1000] + "\n...[truncated]...\n" + err[-900:]
    return f"""# Bead {bead.id} — go test FAILED (pre-merge gate)

You exited cleanly but `go test ./...` from `orchestrator/` is RED. You are
NOT done. Fix the failures below and re-run the gate before stopping.

## go test stdout
```
{out}
```

## go test stderr
```
{err}
```

## Required actions
1. Read the failures above. Common pitfalls: unused imports added without
   call sites, missing constant references, partial sed of legacy literals.
2. Fix every failing package in this worktree (do not touch other beads' scope).
3. Run `cd orchestrator && go vet ./... && go test ./...` until BOTH are clean.
4. If you already committed, AMEND the commit so the bead lands as one clean
   commit: `git add -A && git commit --amend --no-edit`. If you did not commit
   yet, stage and commit with the original conventional message.
5. Stop only when go test ./... is fully green. Do NOT push, do NOT switch
   branches.
"""

# ─── orchestrator (merge + conflict resolution) ────────────────
def _verify_master(bead_id: str) -> tuple[bool, str]:
    """Run go test ./... && go vet ./... on master. Return (ok, output)."""
    gomod_dir = KERNL_REPO / "orchestrator"
    r = subprocess.run(
        ["go", "test", "./..."], cwd=gomod_dir, capture_output=True, text=True,
    )
    if r.returncode != 0:
        return False, f"go test failed:\n{r.stdout}\n{r.stderr}"
    r = subprocess.run(
        ["go", "vet", "./..."], cwd=gomod_dir, capture_output=True, text=True,
    )
    if r.returncode != 0:
        return False, f"go vet failed:\n{r.stdout}\n{r.stderr}"
    return True, ""

def _resolve_regression(responsible_bead_id: str, current_results: list[WorkerResult]) -> bool:
    """
    Invoke litellm/kimi-k2.6 to fix a regression detected after merge.
    Passes full context: failing test output, merged branches, file diffs.
    Returns True if the regression was fixed and tests now pass.
    """
    # Gather context from all branches merged in this round
    context_branches = []
    for r in current_results:
        if not r.success or not r.commit_sha:
            continue
        files_raw = r.bead.metadata.get("files", "(unknown)")
        if isinstance(files_raw, dict):
            f = files_raw
            changed = f.get("create", []) + f.get("modify", [])
            files_summary = json.dumps(changed)
        else:
            files_summary = str(files_raw)
        # Truncate long descriptions for prompt size limits
        desc_lines = r.bead.description.strip().split("\n")
        desc_summary = "\n".join(desc_lines[:12])  # first 12 lines
        if len(desc_lines) > 12:
            desc_summary += "\n  ... (truncated)"
        context_branches.append(
            f"- {r.bead.id} ({r.bead.branch})\n"
            f"  title: {r.bead.title}\n"
            f"  files: {files_summary}\n"
            f"  description:\n{desc_summary}"
        )

    # Get test output
    _, test_output = _verify_master(responsible_bead_id)
    
    # Get HEAD commit message to identify which bead caused the regression
    try:
        head_msg = _git("log", "-1", "--format=%s").stdout.strip()
    except Exception:
        head_msg = "(could not get HEAD)"

    prompt = f"""# Regression Fix Agent

You are the regression fixer for the kernl parallel swarm.
A merge into `master` introduced a regression. Tests are failing.

## Your task

1. `cd {KERNL_REPO}` and ensure you are on `master`.
2. Examine the failing tests:
```
{test_output[:3000]}
```

3. Identify what changed. The merge that likely introduced the regression:
   - Commit: {head_msg}
   - Responsible bead: {responsible_bead_id}

4. Rules for fixing:
   - If a test asserts on old/invalid behavior → update the test.
   - If a function now returns wrong values → fix the function.
   - If two branches added duplicate/conflicting definitions → merge them, keep better one.
   - If imports are broken → fix imports, do NOT rewrite whole files.
   - NEVER introduce new design patterns, abstractions, or refactor unrelated code.
   - The changes should be MINIMAL and focused only on what caused the regression.

5. Commands to run after fixing:
   cd {KERNL_REPO}/orchestrator && go test ./... && go vet ./...
   If this fails, iterate.

6. Commit with:
   git add -A && git commit -m "fix(regression): resolve merge regression from {responsible_bead_id}"

## Merged branches in this round (with descriptions)
{chr(10).join(context_branches)}

## Expected output
Only respond with:
- "FIX_OK" if tests now pass.
- "FIX_FAILED: <reason>" if you cannot resolve.
"""

    log_path = LOGS_DIR / f"regression-fix-{responsible_bead_id}-{datetime.now().strftime('%Y%m%d-%H%M%S')}.log"
    env = os.environ.copy()
    env["OPENCODE_CONFIG"] = str(OPENCODE_CONFIG)
    cmd = [
        "opencode", "run",
        "--format", "json",
        "--dir", str(KERNL_REPO),
        "-m", MODEL_ORCHESTRATOR,
        "-f", str(AGENTS_MD),
        "--title", f"kernl-regression-fix:{responsible_bead_id}",
        prompt,
    ]
    info(f"[{responsible_bead_id}] invoking regression-fix agent ({MODEL_ORCHESTRATOR})…")
    proc = subprocess.Popen(cmd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        stdout, stderr = proc.communicate(timeout=30*60)
    except subprocess.TimeoutExpired:
        proc.kill()
        stdout, stderr = proc.communicate()
        err(f"[{responsible_bead_id}] regression-fix agent timed out")

    with open(log_path, "w", encoding="utf-8") as fh:
        fh.write("=== STDOUT ===\n"); fh.write(stdout)
        fh.write("\n=== STDERR ===\n"); fh.write(stderr)
    info(f"[{responsible_bead_id}] regression-fix log written → {log_path}")

    # Verify after fix
    ok_after, verify_output = _verify_master(responsible_bead_id)
    combined = (stdout + stderr).lower()
    if ok_after and ("fix_ok" in combined or proc.returncode == 0):
        ok(f"[{responsible_bead_id}] regression-fix agent resolved and tests pass")
        return True

    err(f"[{responsible_bead_id}] regression-fix failed or tests still broken:\n{verify_output}")
    err(f"[{responsible_bead_id}] Reverting merge…")
    _git("reset", "--hard", "HEAD~1")
    return False

def _post_merge_gate(bead_id: str, results: list[WorkerResult]) -> bool:
    """Gate: after every merge, verify master still passes tests.
    If not, attempt to auto-fix with the regression agent."""
    info(f"[{bead_id}] running post-merge validation gate…")
    gate_ok, output = _verify_master(bead_id)
    if gate_ok:
        ok(f"[{bead_id}] post-merge gate PASSED")
        return True
    err(f"[{bead_id}] POST-MERGE GATE FAILED — tests broke after merging this bead.\n{output}")
    err(f"KERNL DISPATCH FAILURE: attempting auto-fix for regression from {bead_id}")
    fixed = _resolve_regression(bead_id, results)
    if fixed:
        ok(f"[{bead_id}] regression auto-fixed and gate now passes")
        return True
    err(f"KERNL DISPATCH FAILURE: auto-fix failed — regression remains from {bead_id}")
    err(f"Fix: inspect log, revert with git reset --hard HEAD~1, fix bead, re-run swarm")
    raise RuntimeError(f"post-merge gate failed for {bead_id}")

def merge_batch(results: list[WorkerResult], dry_run: bool) -> list[WorkerResult]:
    """Merge all successful branches into master. Returns successfully-merged results."""
    merged: list[WorkerResult] = []
    for res in results:
        if not res.success or not res.commit_sha:
            warn(f"[{res.bead.id}] skipping merge — no success")
            continue
        if dry_run:
            info(f"[DRY-RUN] would merge {res.bead.branch}")
            merged.append(res)
            continue

        ok_merge, stderr = ff_merge_branch(res.bead.branch)
        if ok_merge:
            _post_merge_gate(res.bead.id, results)
            merged.append(res)
            continue

        warn(f"[{res.bead.id}] ff-merge failed: {stderr.strip()}")
        # Try non-ff merge
        ok_merge2, stderr2 = merge_branch_non_ff(res.bead.branch)
        if ok_merge2:
            _post_merge_gate(res.bead.id, results)
            merged.append(res)
            continue

        warn(f"[{res.bead.id}] merge --no-ff also failed: {stderr2.strip()}")
        # Reset and invoke orchestrator
        reset_merge()
        if resolve_conflicts_with_orchestrator(res.bead, results):
            _post_merge_gate(res.bead.id, results)
            merged.append(res)
        else:
            err(f"[{res.bead.id}] orchestrator could not resolve — leaving branch for manual")
    return merged

def resolve_conflicts_with_orchestrator(failed_bead: Bead, all_results: list[WorkerResult]) -> bool:
    """Call kimi-k2.6 to resolve merge conflicts for one branch."""
    # Build context: which other branches are already merged / pending
    context = []
    for r in all_results:
        if r.bead.id == failed_bead.id:
            continue
        state = "success" if r.success else "failed"
        context.append(f"- {r.bead.branch} (sha={r.commit_sha or 'n/a'}, state={state})")

    prompt = f"""# Merge Orchestrator Task

You are the merge orchestrator for the kernl parallel swarm.
A merge conflict occurred when merging branch `{failed_bead.branch}` into `{MASTER_BRANCH}`.

## Conflicting branch
- Branch: `{failed_bead.branch}`
- Bead: {failed_bead.id} — {failed_bead.title}
- SHA: {failed_bead.branch} latest commit

## Other branches in this batch (already merged or pending)
{chr(10).join(context) if context else "(none)"}

## Your task

1. `cd {KERNL_REPO}`
2. `git checkout {MASTER_BRANCH}`
3. `git merge {failed_bead.branch}`
4. If conflicts exist, examine each conflicted file. Resolve by keeping BOTH
   non-overlapping changes. If the SAME lines were edited by multiple agents,
   choose the implementation that:
   a) Has passing tests (`go test ./...` in `orchestrator/`)
   b) Follows AGENTS.md style rules
   c) Is more complete / less hacky
5. `git add -A && git commit -m "merge: resolve conflicts for {failed_bead.id}"`
6. Run `cd orchestrator && go test ./... && go vet ./...`
7. Report success or failure.

## Rules
- NEVER discard code that has passing tests.
- When in doubt, include both implementations and add a TODO comment.
- Run the full test suite before declaring success.
- Do NOT push.

## Expected output
Respond ONLY with:
- "MERGE_OK" if merge succeeded and tests pass.
- "MERGE_FAILED: <reason>" if you could not resolve.
"""
    env = os.environ.copy()
    env["OPENCODE_CONFIG"] = str(OPENCODE_CONFIG)
    # Run orchestrator in the repo root (not a worktree)
    cmd = [
        "opencode", "run",
        "--format", "json",
        "--dir", str(KERNL_REPO),
        "-m", MODEL_ORCHESTRATOR,
        "-f", str(AGENTS_MD),
        "--title", f"kernl-merge-orchestrator:{failed_bead.id}",
        prompt,
    ]
    info(f"[{failed_bead.id}] invoking orchestrator ({MODEL_ORCHESTRATOR}) for conflict resolution…")
    log_path = LOGS_DIR / f"orchestrator-{failed_bead.id}-{datetime.now().strftime('%Y%m%d-%H%M%S')}.log"
    proc = subprocess.Popen(cmd, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        stdout, stderr = proc.communicate(timeout=30*60)  # 30 min for merge resolution
    except subprocess.TimeoutExpired:
        proc.kill()
        stdout, stderr = proc.communicate()
        err(f"[{failed_bead.id}] orchestrator timed out")
        return False

    # Persist orchestrator output for audit / replay
    with open(log_path, "w", encoding="utf-8") as fh:
        fh.write("=== STDOUT ===\n")
        fh.write(stdout)
        fh.write("\n=== STDERR ===\n")
        fh.write(stderr)
    info(f"[{failed_bead.id}] orchestrator log written → {log_path}")

    combined = (stdout + stderr).lower()
    if "merge_ok" in combined:
        ok(f"[{failed_bead.id}] orchestrator resolved conflicts successfully")
        return True
    err(f"[{failed_bead.id}] orchestrator failed: {stderr[:500]}")
    return False

# ─── batch runner ────────────────────────────────────────────────
import queue as _queue

def run_batch(beads: list[Bead], dry_run: bool, num_workers: int, use_claude: bool = False, resume: bool = False) -> list[WorkerResult]:
    """Dispatch all beads in parallel up to num_workers, return results."""
    q: _queue.Queue[Bead] = _queue.Queue()
    for bead in beads:
        q.put(bead)

    results: list[WorkerResult] = []
    results_mu = threading.Lock()

    def worker_wrapper(worker_idx: int) -> None:
        while True:
            try:
                bead = q.get(block=False)
            except _queue.Empty:
                break
            info(f"[batch] worker {worker_idx + 1}/{num_workers} picked {bead.id}")
            res = run_worker(bead, dry_run, use_claude=use_claude, resume=resume, tailer_lock=tailer_lock)
            with results_mu:
                results.append(res)
            if res.success:
                ok(f"[{bead.id}] worker done — commit {res.commit_sha[:8] if res.commit_sha else 'n/a'}")
            else:
                err(f"[{bead.id}] worker failed — {res.error}")

    tailer_lock = threading.Lock() if _tailer_mod is not None else None
    threads: list[threading.Thread] = []
    for i in range(min(num_workers, len(beads))):
        t = threading.Thread(target=worker_wrapper, args=(i,))
        t.start()
        threads.append(t)

    for t in threads:
        t.join()

    return results

# ─── main ───────────────────────────────────────────────────────
def main() -> int:
    parser = argparse.ArgumentParser(description="kernl parallel swarm")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--max-beads", type=int, default=0, help="0 = unbounded")
    parser.add_argument("--workers", type=int, default=MAX_WORKERS, help=f"parallel workers (default {MAX_WORKERS})")
    parser.add_argument("--claude", action="store_true", help=f"use Claude Code ({MODEL_CLAUDE}) instead of opencode")
    parser.add_argument("--resume", action="store_true", help="skip worktree creation if it already exists (preserves in-progress work)")
    args = parser.parse_args()

    LOGS_DIR.mkdir(parents=True, exist_ok=True)
    WORKTREE_ROOT.mkdir(parents=True, exist_ok=True)

    if not args.dry_run:
        ensure_on_master_clean()

    info(f"kernl parallel swarm — workers={args.workers} max-beads={args.max_beads or 'unbounded'} claude={args.claude}")
    if args.dry_run: warn("DRY RUN")

    processed = 0
    summary = {"success": [], "failed": [], "merged": []}

    while True:
        if args.max_beads and processed >= args.max_beads:
            info(f"reached max-beads={args.max_beads}, stopping")
            break

        beads = fetch_ready_beads()
        if not beads:
            info("no more ready beads — done")
            break

        # Use ALL ready beads — not just args.workers.
        # Workers pick from the queue internally.
        batch = beads
        info(f"━" * 72)
        info(f"round: {[b.id for b in batch]} ({len(batch)} beads, {args.workers} workers)")

        results = run_batch(batch, args.dry_run, args.workers, use_claude=args.claude, resume=args.resume)

        # merge orchestrator phase
        merged = merge_batch(results, args.dry_run)

        for res in merged:
            if not args.dry_run:
                close_bead(res.bead.id, f"Merged by parallel swarm ({res.commit_sha[:8]})")
                remove_worktree(res.bead)
            summary["merged"].append(res.bead.id)
            summary["success"].append(res.bead.id)

        merged_ids = {res.bead.id for res in merged}
        for res in results:
            if res.bead.id not in merged_ids:
                summary["failed"].append(res.bead.id)

        processed += len(batch)

        # stop on failure in non-dry-run to avoid cascading damage
        if summary["failed"] and not args.dry_run:
            warn("stopping after first batch with failures")
            break

    info(f"━" * 72)
    info("SWARM RUN COMPLETE")
    ok(f"  succeeded+merged: {len(summary['success'])}  {summary['success']}")
    if summary["failed"]:  err(f"  failed:  {summary['failed']}")
    return 0 if not summary["failed"] else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        warn("interrupted — worktrees retained for inspection")
        sys.exit(130)
