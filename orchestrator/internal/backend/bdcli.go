package backend

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	outOfSyncSignature     = "Database out of sync with JSONL"
	noDaemonFlag           = "--no-daemon"
	bdNoDBEnv              = "BD_NO_DB"
	doltNilPanicSignature  = "panic: runtime error: invalid memory address or nil pointer dereference"
	doltPanicStackSignature = "SetCrashOnFatalError"
	lockWaitTimeoutSig     = "Timed out waiting for bd repo lock"
	commandTimeoutSig      = "bd command timed out after"

	defaultCommandTimeoutMs = 5000
	defaultLockStaleMs      = 600000

	maxTimeoutRetries = 1
)

var (
	readOnlyBdCommands = map[string]bool{
		"list":   true,
		"ready":  true,
		"search": true,
		"query":  true,
		"show":   true,
	}
)

// withRepo prepends bd's global `-C <path>` flag to a command's args. bd 1.0+
// removed the per-subcommand `--repo` flag in favor of this directory selector.
func withRepo(repoPath string, args ...string) []string {
	out := make([]string, 0, len(args)+2)
	out = append(out, "-C", repoPath)
	out = append(out, args...)
	return out
}

// stripRepoPrefix returns args with a leading `-C <path>` stripped, so
// IsReadOnlyCommand / isIdempotentWriteCommand can keep classifying by subcommand.
func stripRepoPrefix(args []string) []string {
	if len(args) >= 2 && args[0] == "-C" {
		return args[2:]
	}
	return args
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

type ExecOptions struct {
	Cwd        string
	ForceNoDB  bool
	Env        []string
	TimeoutMs  int
}

type repoQueue struct {
	tail     chan struct{}
	pending  int
}

type BdCliBackend struct {
	repoPath string
	mu       sync.Mutex

	queues   map[string]*repoQueue
	queueMu  sync.Mutex

	locksDir string
	bdBin    string
	bdDB     string
}

func NewBdCliBackend(repoPath string) *BdCliBackend {
	locksDir := os.Getenv("KERNL_BD_LOCK_DIR")
	if locksDir == "" {
		locksDir = filepath.Join(os.TempDir(), "kernl-bd-locks")
	}
	bdBin := os.Getenv("BD_BIN")
	if bdBin == "" {
		bdBin = "bd"
	}
	return &BdCliBackend{
		repoPath: repoPath,
		queues:   make(map[string]*repoQueue),
		locksDir: locksDir,
		bdBin:    bdBin,
		bdDB:     os.Getenv("BD_DB"),
	}
}

func (b *BdCliBackend) Capabilities() BackendCapabilities {
	return FullCapabilities
}

func (b *BdCliBackend) ListWorkflows(repoPath string) ([]WorkflowDescriptor, error) {
	out, err := b.Exec(context.Background(), []string{"list-workflows", "--json"})
	if err != nil {
		return nil, fmt.Errorf("bd list-workflows: %w", err)
	}
	var result []WorkflowDescriptor
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("bd list-workflows parse: %w", err)
	}
	return result, nil
}

func (b *BdCliBackend) List(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	args := withRepo(repoPath, "list", "--json")
	if filters != nil {
		if filters.State == "" {
			args = append(args, "--all")
		} else {
			args = append(args, "--status", filters.State)
		}
		if filters.Type != "" {
			args = append(args, "--type", filters.Type)
		}
		if filters.Label != "" {
			args = append(args, "--label", filters.Label)
		}
		if filters.Assignee != "" {
			args = append(args, "--assignee", filters.Assignee)
		}
		if filters.Priority != 0 {
			args = append(args, "--priority", fmt.Sprintf("%d", filters.Priority))
		}
		if filters.Parent != "" {
			args = append(args, "--parent", filters.Parent)
		}
	} else {
		args = append(args, "--all")
	}
	out, err := b.Exec(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("bd list: %w", err)
	}
	var raw []RawBead
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("bd list parse: %w", err)
	}
	result := make([]Bead, 0, len(raw))
	for _, r := range raw {
		result = append(result, NormalizeBead(r))
	}
	return result, nil
}

func (b *BdCliBackend) ListReady(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	readyFilters := &BeadListFilters{}
	if filters != nil {
		*readyFilters = *filters
	}
	readyFilters.State = "ready_for_implementation"
	return b.List(readyFilters, repoPath)
}

func (b *BdCliBackend) Get(id string, repoPath string) (*Bead, error) {
	out, err := b.Exec(context.Background(), withRepo(repoPath, "show", id, "--json"))
	if err != nil {
		return nil, fmt.Errorf("bd show %s: %w", id, err)
	}
	var raw RawBead
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("bd show parse: %w", err)
	}
	bead := NormalizeBead(raw)
	return &bead, nil
}

func (b *BdCliBackend) Create(input CreateBeadInput, repoPath string) (*Bead, error) {
	args := withRepo(repoPath, "create", input.Title, "--json")
	if input.Type != "" {
		args = append(args, "--type", input.Type)
	}
	if input.ParentID != "" {
		args = append(args, "--parent", input.ParentID)
	}
	result, err := b.exec(context.Background(), args, nil)
	if err != nil {
		return nil, fmt.Errorf("bd create: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, bdResultToError(result)
	}
	var bead Bead
	if err := json.Unmarshal([]byte(result.Stdout), &bead); err != nil {
		id := strings.TrimSpace(result.Stdout)
		if id != "" {
			return &Bead{ID: id}, nil
		}
		return nil, fmt.Errorf("bd create parse: %w", err)
	}
	return &bead, nil
}

func (b *BdCliBackend) Update(id string, input UpdateBeadInput, repoPath string) error {
	args := withRepo(repoPath, "update", id, "--json")
	if input.Title != "" {
		args = append(args, "--title", input.Title)
	}
	if input.Description != "" {
		args = append(args, "--description", input.Description)
	}
	if input.Type != "" {
		args = append(args, "--type", input.Type)
	}
	if input.State != "" {
		args = append(args, "--status", input.State)
	}
	if input.Priority != nil {
		args = append(args, "--priority", fmt.Sprintf("%d", *input.Priority))
	}
	if input.Assignee != "" {
		args = append(args, "--assignee", input.Assignee)
	}
	if input.Acceptance != "" {
		args = append(args, "--acceptance", input.Acceptance)
	}
	if len(input.Labels) > 0 {
		for _, l := range input.Labels {
			args = append(args, "--label", l)
		}
	}
	_, err := b.Exec(context.Background(), args)
	if err != nil {
		return fmt.Errorf("bd update %s: %w", id, err)
	}
	return nil
}

func (b *BdCliBackend) Close(id string, reason string, repoPath string) (*TerminalState, error) {
	args := withRepo(repoPath, "close", id, "--json")
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	out, err := b.Exec(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("bd close %s: %w", id, err)
	}
	var result TerminalState
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("bd close parse: %w", err)
	}
	return &result, nil
}

func (b *BdCliBackend) Delete(id string, repoPath string) error {
	_, err := b.Exec(context.Background(), withRepo(repoPath, "delete", id, "--json", "--force"))
	if err != nil {
		return fmt.Errorf("bd delete %s: %w", id, err)
	}
	return nil
}

func (b *BdCliBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	args := withRepo(repoPath, "update", id, "--status", targetState, "--force")
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	_, err := b.Exec(context.Background(), args)
	if err != nil {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: mark terminal %s -> %s: %w", id, targetState, err)
	}
	return nil
}

func (b *BdCliBackend) Reopen(id string, reason string, repoPath string) error {
	return fmt.Errorf("KERNL DISPATCH FAILURE: bd backend does not support reopen; use knots backend for retake flows")
}

func (b *BdCliBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return fmt.Errorf("KERNL DISPATCH FAILURE: bd backend does not support rewind; use knots backend for workflow corrections")
}

func (b *BdCliBackend) Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error) {
	args := withRepo(repoPath, "search", query, "--json")
	if filters != nil {
		if filters.Priority != 0 {
			args = append(args, "--priority-min", fmt.Sprintf("%d", filters.Priority), "--priority-max", fmt.Sprintf("%d", filters.Priority))
		}
	}
	out, err := b.Exec(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("bd search: %w", err)
	}
	var result []Bead
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("bd search parse: %w", err)
	}
	return result, nil
}

func (b *BdCliBackend) Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error) {
	args := withRepo(repoPath, "query", expression, "--json")
	if options != nil {
		if options.Limit > 0 {
			args = append(args, "--limit", fmt.Sprintf("%d", options.Limit))
		}
		if options.Sort != "" {
			args = append(args, "--sort", options.Sort)
		}
	}
	out, err := b.Exec(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("bd query: %w", err)
	}
	var result []Bead
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("bd query parse: %w", err)
	}
	return result, nil
}

func (b *BdCliBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	_, err := b.Exec(context.Background(), withRepo(repoPath, "add-dep", blockerID, blockedID, "--json"))
	if err != nil {
		return fmt.Errorf("bd add-dep: %w", err)
	}
	return nil
}

func (b *BdCliBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	_, err := b.Exec(context.Background(), withRepo(repoPath, "remove-dep", blockerID, blockedID, "--json"))
	if err != nil {
		return fmt.Errorf("bd remove-dep: %w", err)
	}
	return nil
}

func (b *BdCliBackend) ListDependencies(id string, repoPath string, options *DependencyListOptions) ([]BeadDependency, error) {
	args := withRepo(repoPath, "list-deps", id, "--json")
	if options != nil && options.Type != "" {
		args = append(args, "--type", options.Type)
	}
	out, err := b.Exec(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("bd list-deps: %w", err)
	}
	var result []BeadDependency
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("bd list-deps parse: %w", err)
	}
	return result, nil
}

func (b *BdCliBackend) BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error) {
	return nil, fmt.Errorf("KERNL DISPATCH FAILURE: bd backend does not support buildTakePrompt; use scope refinement worker")
}

func (b *BdCliBackend) BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error) {
	return nil, fmt.Errorf("KERNL DISPATCH FAILURE: bd backend does not support buildPollPrompt; use scope refinement worker")
}

func (b *BdCliBackend) ExecWithNoDaemonFallback(ctx context.Context, args []string) (json.RawMessage, error) {
	result, err := b.exec(ctx, args, nil)
	if err != nil {
		return nil, err
	}
	if result.ExitCode == 0 {
		return parseNDJSONBytes([]byte(result.Stdout))
	}

	if !containsNoDaemonFlag(args) || !isUnknownNoDaemonFlagError(result) {
		return nil, bdResultToError(result)
	}

	stripped := stripNoDaemonFlag(args)
	result2, err := b.exec(ctx, stripped, nil)
	if err != nil {
		return nil, err
	}
	if result2.ExitCode != 0 {
		return nil, bdResultToError(result2)
	}
	return parseNDJSONBytes([]byte(result2.Stdout))
}

func IsReadOnlyCommand(args []string) bool {
	args = stripRepoPrefix(args)
	if len(args) == 0 {
		return false
	}
	if args[0] == "dep" && len(args) > 1 && args[1] == "list" {
		return true
	}
	return readOnlyBdCommands[args[0]]
}

func isIdempotentWriteCommand(args []string) bool {
	if IsReadOnlyCommand(args) {
		return false
	}
	args = stripRepoPrefix(args)
	if len(args) == 0 {
		return false
	}
	if args[0] == "update" {
		return true
	}
	if args[0] == "label" && len(args) > 1 && (args[1] == "add" || args[1] == "remove") {
		return true
	}
	if args[0] == "sync" {
		return true
	}
	if args[0] == "dep" && len(args) > 1 && args[1] == "remove" {
		return true
	}
	return false
}

func canRetryAfterTimeout(args []string) bool {
	return IsReadOnlyCommand(args) || isIdempotentWriteCommand(args)
}

func shouldUseNoDBByDefault(args []string) bool {
	if isTruthyEnv(bdNoDBEnv) {
		return true
	}
	if os.Getenv("KERNL_BD_READ_NO_DB") == "0" {
		return false
	}
	return IsReadOnlyCommand(args)
}

func isTruthyEnv(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "1" || v == "true" || v == "yes"
}

func commandTimeoutMs(args []string) int {
	envKey := "KERNL_BD_COMMAND_TIMEOUT_MS"
	if IsReadOnlyCommand(args) {
		envKey = "KERNL_BD_READ_TIMEOUT_MS"
	}
	if v := os.Getenv(envKey); v != "" {
		if n := parseIntEnv(v); n > 0 {
			return n
		}
	}
	if v := os.Getenv("KERNL_BD_COMMAND_TIMEOUT_MS"); v != "" {
		if n := parseIntEnv(v); n > 0 {
			return n
		}
	}
	return defaultCommandTimeoutMs
}

func parseIntEnv(v string) int {
	var n int
	fmt.Sscanf(v, "%d", &n)
	return n
}

func (b *BdCliBackend) Exec(ctx context.Context, args []string) (json.RawMessage, error) {
	result, err := b.exec(ctx, args, nil)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, bdResultToError(result)
	}
	return parseNDJSONBytes([]byte(result.Stdout))
}

func (b *BdCliBackend) exec(ctx context.Context, args []string, opts *ExecOptions) (*ExecResult, error) {
	maxAttempts := 1
	if canRetryAfterTimeout(args) {
		maxAttempts = 1 + maxTimeoutRetries
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := b.execSerializedAttempt(ctx, args, opts)
		if err != nil {
			msg := "Failed to run bd command"
			if e, ok := err.(*bdExecError); ok {
				msg = e.Error()
			}
			result = &ExecResult{
				Stdout:   "",
				Stderr:   msg,
				ExitCode: 1,
				TimedOut: strings.Contains(msg, lockWaitTimeoutSig),
			}
			if !canRetryAfterTimeout(args) || attempt >= maxAttempts {
				return result, nil
			}
			if isTimeoutResult(result) {
				slog.Debug("bd timeout, retrying", "attempt", attempt, "args", args)
				continue
			}
			return result, nil
		}

		if result.ExitCode == 0 {
			return result, nil
		}

		shouldRetry := attempt < maxAttempts && isTimeoutResult(result)
		if shouldRetry {
			slog.Debug("bd timeout, retrying", "attempt", attempt, "args", args)
			continue
		}
		return result, nil
	}

	return &ExecResult{
		Stdout:   "",
		Stderr:   "Failed to run bd command",
		ExitCode: 1,
		TimedOut: false,
	}, nil
}

func (b *BdCliBackend) execSerializedAttempt(ctx context.Context, args []string, opts *ExecOptions) (*ExecResult, error) {
	repoKey := b.repoPath
	if opts != nil && opts.Cwd != "" {
		repoKey = opts.Cwd
	}

	result, err := b.withRepoSerialization(ctx, repoKey, func() (*ExecResult, error) {
		useNoDB := shouldUseNoDBByDefault(args)
		if opts != nil && opts.ForceNoDB {
			useNoDB = true
		}
		execOpts := &ExecOptions{
			Cwd:       resolveCwd(opts),
			ForceNoDB: useNoDB,
		}

		firstResult, err := execOnce(ctx, b.bdBin, b.bdDB, args, execOpts)
		if err != nil {
			return nil, err
		}
		if firstResult.ExitCode == 0 {
			return firstResult, nil
		}

		if !useNoDB && IsReadOnlyCommand(args) && isEmbeddedDoltPanic(firstResult) {
			retryOpts := &ExecOptions{
				Cwd:       execOpts.Cwd,
				ForceNoDB: true,
			}
			return execOnce(ctx, b.bdBin, b.bdDB, args, retryOpts)
		}

		if subcmdArgs := stripRepoPrefix(args); len(subcmdArgs) > 0 && subcmdArgs[0] == "sync" || !isOutOfSyncError(firstResult) {
			return firstResult, nil
		}

		syncResult, syncErr := execOnce(ctx, b.bdBin, b.bdDB, []string{"sync", "--import-only"}, execOpts)
		if syncErr != nil {
			return firstResult, nil
		}
		if syncResult.ExitCode != 0 {
			return firstResult, nil
		}
		return execOnce(ctx, b.bdBin, b.bdDB, args, execOpts)
	})

	return result, err
}

func execOnce(ctx context.Context, bdBin string, bdDB string, args []string, opts *ExecOptions) (*ExecResult, error) {
	fullArgs := baseArgs(bdDB, args)

	timeoutMs := defaultCommandTimeoutMs
	if opts != nil && opts.TimeoutMs > 0 {
		timeoutMs = opts.TimeoutMs
	} else {
		timeoutMs = commandTimeoutMs(args)
	}

	cmd := exec.CommandContext(ctx, bdBin, fullArgs...)
	if opts != nil && opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}

	cmd.Env = os.Environ()
	if opts != nil && opts.ForceNoDB {
		cmd.Env = append(cmd.Env, bdNoDBEnv+"=true")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd = exec.CommandContext(timeoutCtx, bdBin, fullArgs...)
	if opts != nil && opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	cmd.Env = os.Environ()
	if opts != nil && opts.ForceNoDB {
		cmd.Env = append(cmd.Env, bdNoDBEnv+"=true")
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	timedOut := false
	exitCode := 0

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if timeoutCtx.Err() == context.DeadlineExceeded {
				timedOut = true
				timeoutMsg := fmt.Sprintf("bd command timed out after %dms", timeoutMs)
				stderrStr = timeoutMsg + "\n" + stderrStr
			}
		} else {
			exitCode = 1
			if timeoutCtx.Err() == context.DeadlineExceeded {
				timedOut = true
				timeoutMsg := fmt.Sprintf("bd command timed out after %dms", timeoutMs)
				stderrStr = timeoutMsg + "\n" + stderrStr
			}
		}
	}

	slog.Debug("bd exec completed", "args", args, "exitCode", exitCode, "timedOut", timedOut)

	return &ExecResult{
		Stdout:   strings.TrimSpace(stdoutStr),
		Stderr:   strings.TrimSpace(stderrStr),
		ExitCode: exitCode,
		TimedOut: timedOut,
	}, nil
}

func baseArgs(bdDB string, args []string) []string {
	result := []string{}
	if bdDB != "" {
		result = append(result, "--db", bdDB)
	}
	return append(result, args...)
}

func resolveCwd(opts *ExecOptions) string {
	if opts != nil && opts.Cwd != "" {
		return opts.Cwd
	}
	return ""
}

func (b *BdCliBackend) withRepoSerialization(ctx context.Context, repoKey string, fn func() (*ExecResult, error)) (*ExecResult, error) {
	b.queueMu.Lock()
	q, exists := b.queues[repoKey]
	if !exists {
		q = &repoQueue{tail: make(chan struct{}, 1), pending: 0}
		q.tail <- struct{}{}
		b.queues[repoKey] = q
	}
	q.pending++
	b.queueMu.Unlock()

	defer func() {
		b.queueMu.Lock()
		q.pending--
		if q.pending == 0 {
			delete(b.queues, repoKey)
		}
		b.queueMu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-q.tail:
	}

	releaseLock, lockErr := b.acquireRepoProcessLock(ctx, repoKey)
	defer func() {
		if releaseLock != nil {
			releaseLock()
		}
		b.queueMu.Lock()
		if q, ok := b.queues[repoKey]; ok {
			q.tail <- struct{}{}
		}
		b.queueMu.Unlock()
	}()

	if lockErr != nil {
		return nil, lockErr
	}

	return fn()
}

type lockOwner struct {
	PID        int    `json:"pid"`
	RepoPath   string `json:"repoPath"`
	AcquiredAt string `json:"acquiredAt"`
}

func (b *BdCliBackend) acquireRepoProcessLock(ctx context.Context, repoKey string) (func(), error) {
	digest := fmt.Sprintf("%x", sha1.Sum([]byte(repoKey)))
	lockDir := filepath.Join(b.locksDir, digest)
	lockFile := filepath.Join(lockDir, "owner.json")

	if err := os.MkdirAll(b.locksDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating locks root: %w", err)
	}

	waitStart := time.Now()
	lockWaitTimeoutMs := commandTimeoutMs([]string{"list"})

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			owner := lockOwner{
				PID:        os.Getpid(),
				RepoPath:   repoKey,
				AcquiredAt: time.Now().Format(time.RFC3339),
			}
			data, _ := json.Marshal(owner)
			if writeErr := os.WriteFile(lockFile, data, 0o644); writeErr != nil {
				os.RemoveAll(lockDir)
				return nil, fmt.Errorf("writing lock owner: %w", writeErr)
			}
			release := func() {
				os.RemoveAll(lockDir)
			}
			return release, nil
		}

		if !os.IsExist(err) {
			return nil, fmt.Errorf("creating lock dir: %w", err)
		}

		if b.evictStaleLock(lockDir, lockFile) {
			continue
		}

		if time.Since(waitStart) >= time.Duration(lockWaitTimeoutMs)*time.Millisecond {
			owner := readLockOwner(lockFile)
			ownerDetails := ""
			if owner != nil {
				ownerDetails = fmt.Sprintf(" (owner pid=%d, acquiredAt=%s)", owner.PID, owner.AcquiredAt)
			}
			return nil, fmt.Errorf("Timed out waiting for bd repo lock for %s after %dms%s",
				repoKey, lockWaitTimeoutMs, ownerDetails)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func (b *BdCliBackend) evictStaleLock(lockDir, lockFile string) bool {
	owner := readLockOwner(lockFile)
	if owner != nil && !isPidAlive(owner.PID) {
		os.RemoveAll(lockDir)
		return true
	}

	info, err := os.Stat(lockDir)
	if err != nil {
		if os.IsNotExist(err) {
			return true
		}
		return false
	}

	if time.Since(info.ModTime()) > time.Duration(defaultLockStaleMs)*time.Millisecond {
		os.RemoveAll(lockDir)
		return true
	}

	return false
}

func readLockOwner(lockFile string) *lockOwner {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return nil
	}
	var owner lockOwner
	if err := json.Unmarshal(data, &owner); err != nil {
		return nil
	}
	if owner.PID == 0 || owner.RepoPath == "" || owner.AcquiredAt == "" {
		return nil
	}
	return &owner
}

func isPidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func isEmbeddedDoltPanic(result *ExecResult) bool {
	combined := result.Stderr + "\n" + result.Stdout
	return strings.Contains(combined, doltNilPanicSignature) ||
		strings.Contains(combined, doltPanicStackSignature)
}

func isOutOfSyncError(result *ExecResult) bool {
	combined := result.Stderr + "\n" + result.Stdout
	return strings.Contains(combined, outOfSyncSignature)
}

func isUnknownNoDaemonFlagError(result *ExecResult) bool {
	combined := result.Stderr + "\n" + result.Stdout
	return strings.Contains(combined, "unknown flag: "+noDaemonFlag)
}

func isTimeoutResult(result *ExecResult) bool {
	return result.TimedOut || strings.Contains(result.Stderr, commandTimeoutSig)
}

func containsNoDaemonFlag(args []string) bool {
	for _, arg := range args {
		if arg == noDaemonFlag {
			return true
		}
	}
	return false
}

func stripNoDaemonFlag(args []string) []string {
	var result []string
	for _, arg := range args {
		if arg != noDaemonFlag {
			result = append(result, arg)
		}
	}
	return result
}

type bdExecError struct {
	msg string
}

func (e *bdExecError) Error() string { return e.msg }

func bdResultToError(result *ExecResult) error {
	return fmt.Errorf("bd exit %d: %s", result.ExitCode, result.Stderr)
}

// bd show returns {"..."} (single object)  OR  [{...}] (array with single element)
// bd list returns [{...}, {...}, ...] (array with multiple elements)
// parseNDJSONBytes normalizes all to a single valid JSON document — object, array, or encoded-raw.
func parseNDJSONBytes(data []byte) (json.RawMessage, error) {
	if len(data) == 0 {
		return data, nil
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return data, nil
	}

	// fast path: pretty-printed JSON array
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err == nil {
			if len(arr) == 1 {
				return arr[0], nil // unwrap single-element array (bd show)
			}
			// multi-element array (bd list)
			encoded, _ := json.Marshal(arr)
			return json.RawMessage(encoded), nil
		}
	}

	// object
	if trimmed[0] == '{' {
		var raw json.RawMessage
		if err := json.Unmarshal(trimmed, &raw); err == nil {
			return raw, nil
		}
	}

	// fallback: NDJSON line parser
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var results []json.RawMessage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			slog.Debug("skipping non-JSON line from bd", "line", line)
			continue
		}
		results = append(results, raw)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning bd output: %w", err)
	}

	if len(results) == 1 {
		return results[0], nil
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("marshaling bd results: %w", err)
	}
	return json.RawMessage(encoded), nil
}

func parseNDJSONOutput(data []byte) ([]byte, error) {
	raw, err := parseNDJSONBytes(data)
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}