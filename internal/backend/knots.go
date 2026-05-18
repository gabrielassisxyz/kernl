package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	knoCommandTimeoutMs = 20000
	knoMaxBufferBytes    = 10 * 1024 * 1024
	knoDefaultBin        = "kno"

	knoRetriableLock    = "database is locked"
	knoRetriableBusy    = "busy"
	knoRetriableTimeout = "timed out"
)

var knoRetryDelays = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

type KnotsBackend struct {
	repoPath string
	knoBin   string
	knoDB    string

	writeQueues   map[string]*knoQueue
	writeQueueMu  sync.Mutex
	nextQueues    map[string]*knoQueue
	nextQueueMu   sync.Mutex
}

type knoQueue struct {
	tail    chan struct{}
	pending int
}

type knoExecResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func NewKnotsBackend(repoPath string) *KnotsBackend {
	knoBin := os.Getenv("KNOTS_BIN")
	if knoBin == "" {
		knoBin = knoDefaultBin
	}
	return &KnotsBackend{
		repoPath:    repoPath,
		knoBin:      knoBin,
		knoDB:       os.Getenv("KNOTS_DB_PATH"),
		writeQueues: make(map[string]*knoQueue),
		nextQueues:  make(map[string]*knoQueue),
	}
}

func (k *KnotsBackend) ListWorkflows(repoPath string) ([]WorkflowDescriptor, error) {
	result, err := k.execRead(context.Background(), []string{"workflow", "list", "--json"})
	if err != nil {
		return nil, fmt.Errorf("kno workflow list: %w", err)
	}
	if result.exitCode != 0 {
		fallback, fbErr := k.execRead(context.Background(), []string{"workflow", "ls", "--json"})
		if fbErr != nil || fallback.exitCode != 0 {
			return nil, fmt.Errorf("kno workflow list: %s", firstNonEmpty(result.stderr, "failed"))
		}
		result = fallback
	}
	var workflows []knoWorkflowDefinition
	if err := json.Unmarshal([]byte(result.stdout), &workflows); err != nil {
		return nil, fmt.Errorf("kno workflow list parse: %w", err)
	}
	descriptors := make([]WorkflowDescriptor, len(workflows))
	for i, wf := range workflows {
		queueActions := make(map[string]string)
		for _, t := range wf.Transitions {
			queueActions[t.From] = t.To
		}
		if wf.InitialState != "" {
			queueActions[wf.InitialState] = wf.InitialState
		}
		descriptors[i] = WorkflowDescriptor{
			ID:               wf.ID,
			Label:            wf.ID,
			BackingWorkflowID: wf.ID,
			InitialState:     wf.InitialState,
			States:           wf.States,
			TerminalStates:   wf.TerminalStates,
			QueueActions:     queueActions,
		}
	}
	return descriptors, nil
}

func (k *KnotsBackend) List(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	args := []string{"ls", "--all", "--json"}
	if filters != nil && filters.State != "" {
		args = []string{"ls", "--json", "--status", filters.State}
	}
	result, err := k.execRead(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("kno ls: %w", err)
	}
	if result.exitCode != 0 {
		fallback, fbErr := k.execRead(context.Background(), []string{"ls", "--json"})
		if fbErr != nil || fallback.exitCode != 0 {
			return nil, fmt.Errorf("kno ls: %s", firstNonEmpty(result.stderr, "failed"))
		}
		result = fallback
	}
	var records []knoRecord
	if err := json.Unmarshal([]byte(result.stdout), &records); err != nil {
		return nil, fmt.Errorf("kno ls parse: %w", err)
	}
	beads := make([]Bead, len(records))
	for i, rec := range records {
		beads[i] = knotRecordToBead(rec, repoPath)
	}
	return beads, nil
}

func (k *KnotsBackend) ListReady(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	readyFilters := &BeadListFilters{}
	if filters != nil {
		*readyFilters = *filters
	}
	readyFilters.State = "ready_for_implementation"
	return k.List(readyFilters, repoPath)
}

func (k *KnotsBackend) Get(id string, repoPath string) (*Bead, error) {
	result, err := k.execRead(context.Background(), []string{"show", id, "--json"})
	if err != nil {
		return nil, fmt.Errorf("kno show %s: %w", id, err)
	}
	if result.exitCode != 0 {
		return nil, fmt.Errorf("kno show %s: %s", id, firstNonEmpty(result.stderr, "not found"))
	}
	var rec knoRecord
	if err := json.Unmarshal([]byte(result.stdout), &rec); err != nil {
		return nil, fmt.Errorf("kno show parse: %w", err)
	}
	bead := knotRecordToBead(rec, repoPath)
	return &bead, nil
}

func (k *KnotsBackend) Create(input CreateBeadInput, repoPath string) (*Bead, error) {
	args := []string{"new"}
	if input.Type != "" {
		args = append(args, "--type", input.Type)
	}
	if input.Acceptance != "" {
		args = append(args, "--acceptance="+input.Acceptance)
	}
	if input.ParentID != "" {
		args = append(args, "--parent", input.ParentID)
	}
	args = append(args, "--", input.Title)

	result, err := k.execWrite(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("kno new: %w", err)
	}
	if result.exitCode != 0 {
		return nil, fmt.Errorf("kno new: %s", firstNonEmpty(result.stderr, "failed"))
	}

	createdID := strings.TrimSpace(result.stdout)
	if createdID == "" {
		parts := strings.Fields(result.stdout)
		if len(parts) >= 2 && strings.HasPrefix(parts[0], "created") {
			createdID = parts[1]
		}
	}
	if createdID == "" {
		createdID = strings.TrimSpace(strings.TrimPrefix(result.stdout, "created"))
	}

	bead, getErr := k.Get(createdID, repoPath)
	if getErr != nil {
		return &Bead{ID: createdID, Title: input.Title, Type: input.Type}, nil
	}
	return bead, nil
}

func (k *KnotsBackend) Update(id string, input UpdateBeadInput, repoPath string) error {
	args := []string{"update", id}
	if input.Title != "" {
		args = append(args, fmt.Sprintf("--title=%s", input.Title))
	}
	if input.Description != "" {
		args = append(args, fmt.Sprintf("--description=%s", input.Description))
	}
	if input.Acceptance != "" {
		args = append(args, fmt.Sprintf("--acceptance=%s", input.Acceptance))
	}
	if input.State != "" {
		args = append(args, fmt.Sprintf("--status=%s", input.State))
	}
	if input.Type != "" {
		args = append(args, fmt.Sprintf("--type=%s", input.Type))
	}
	if input.Priority != nil {
		args = append(args, "--priority", fmt.Sprintf("%d", *input.Priority))
	}
	if input.Assignee != "" {
		args = append(args, fmt.Sprintf("--assignee=%s", input.Assignee))
	}
	if input.Due != "" {
		args = append(args, fmt.Sprintf("--due=%s", input.Due))
	}
	if len(input.Labels) > 0 {
		for _, tag := range input.Labels {
			if strings.TrimSpace(tag) != "" {
				args = append(args, "--add-tag="+tag)
			}
		}
	}
	if len(input.RemoveLabels) > 0 {
		for _, tag := range input.RemoveLabels {
			if strings.TrimSpace(tag) != "" {
				args = append(args, "--remove-tag="+tag)
			}
		}
	}
	if input.Notes != "" {
		args = append(args, fmt.Sprintf("--add-note=%s", input.Notes))
		args = append(args, "--note-username", "kernl")
	}
	result, err := k.execWrite(context.Background(), args)
	if err != nil {
		return fmt.Errorf("kno update %s: %w", id, err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("kno update %s: %s", id, firstNonEmpty(result.stderr, "failed"))
	}
	return nil
}

func (k *KnotsBackend) Close(id string, reason string, repoPath string) (*TerminalState, error) {
	args := []string{"next", id}
	if reason != "" {
		args = append(args, "--expected-state", reason)
	}
	result, err := k.execWrite(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("kno close %s: %w", id, err)
	}
	if result.exitCode != 0 {
		return nil, fmt.Errorf("kno close %s: %s", id, firstNonEmpty(result.stderr, "failed"))
	}
	return &TerminalState{State: "shipped", Reason: reason}, nil
}

func (k *KnotsBackend) Delete(id string, repoPath string) error {
	return fmt.Errorf("KERNL DISPATCH FAILURE: knots backend does not support delete; use update to close instead")
}

func (k *KnotsBackend) Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error) {
	all, err := k.List(filters, repoPath)
	if err != nil {
		return nil, err
	}
	var matches []Bead
	lowerQ := strings.ToLower(query)
	for i := range all {
		if strings.Contains(strings.ToLower(all[i].Title), lowerQ) || strings.Contains(strings.ToLower(all[i].ID), lowerQ) {
			matches = append(matches, all[i])
		}
	}
	return matches, nil
}

func (k *KnotsBackend) Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error) {
	return k.List(nil, repoPath)
}

func (k *KnotsBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	args := []string{"update", id, "--force", "--status", targetState}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	result, err := k.execWrite(context.Background(), args)
	if err != nil {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: mark terminal %s -> %s: %w", id, targetState, err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: mark terminal %s -> %s: %s", id, targetState, firstNonEmpty(result.stderr, "failed"))
	}
	return nil
}

func (k *KnotsBackend) Reopen(id string, reason string, repoPath string) error {
	args := []string{"next", id, "--force"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	result, err := k.execWrite(context.Background(), args)
	if err != nil {
		return fmt.Errorf("kno reopen %s: %w", id, err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("kno reopen %s: %s", id, firstNonEmpty(result.stderr, "failed"))
	}
	return nil
}

func (k *KnotsBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	if !strings.HasPrefix(targetState, "ready_for_") {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: rewind target %q must be a queue state (ready_for_*)", targetState)
	}
	args := []string{"update", id, "--force", "--status", targetState}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	result, err := k.execWrite(context.Background(), args)
	if err != nil {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: rewind %s -> %s: %w", id, targetState, err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: rewind %s -> %s: %s", id, targetState, firstNonEmpty(result.stderr, "failed"))
	}
	return nil
}

func (k *KnotsBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	result, err := k.execWrite(context.Background(), []string{"edge", "add", blockerID, "blocked_by", blockedID})
	if err != nil {
		return fmt.Errorf("kno edge add: %w", err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("kno edge add: %s", firstNonEmpty(result.stderr, "failed"))
	}
	return nil
}

func (k *KnotsBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	result, err := k.execWrite(context.Background(), []string{"edge", "remove", blockerID, "blocked_by", blockedID})
	if err != nil {
		return fmt.Errorf("kno edge remove: %w", err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("kno edge remove: %s", firstNonEmpty(result.stderr, "failed"))
	}
	return nil
}

func (k *KnotsBackend) ListDependencies(id string, repoPath string, options *DependencyListOptions) ([]BeadDependency, error) {
	args := []string{"edge", "list", id, "--direction", "both", "--json"}
	if options != nil && options.Type != "" {
		args = append(args, "--type", options.Type)
	}
	result, err := k.execRead(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("kno edge list: %w", err)
	}
	if result.exitCode != 0 {
		return nil, fmt.Errorf("kno edge list: %s", firstNonEmpty(result.stderr, "failed"))
	}
	var edges []knoEdge
	if err := json.Unmarshal([]byte(result.stdout), &edges); err != nil {
		return nil, fmt.Errorf("kno edge list parse: %w", err)
	}
	deps := make([]BeadDependency, len(edges))
	for i, e := range edges {
		depType := e.Kind
		if e.Kind == "blocked_by" {
			depType = "blocks"
		} else if e.Kind == "parent_of" {
			depType = "parent-child"
		}
		deps[i] = BeadDependency{SourceID: e.Src, TargetID: e.Dst, Type: depType}
	}
	return deps, nil
}

func (k *KnotsBackend) BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error) {
	return nil, fmt.Errorf("KERNL DISPATCH FAILURE: knots backend buildTakePrompt not yet implemented; use scope refinement worker")
}

type CreateLeaseOptions struct {
	Nickname     string `json:"nickname"`
	Type         string `json:"type,omitempty"`
	AgentName    string `json:"agentName,omitempty"`
	AgentType    string `json:"agentType,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	ModelVersion string `json:"modelVersion,omitempty"`
}

type LeaseResult struct {
	ID    string      `json:"id"`
	Lease *knoLease   `json:"lease,omitempty"`
}

func (k *KnotsBackend) CreateLease(opts CreateLeaseOptions, repoPath string) (*LeaseResult, error) {
	args := []string{"lease", "create", "--nickname", opts.Nickname}
	if opts.Type != "" {
		args = append(args, "--type", opts.Type)
	}
	if opts.AgentName != "" {
		args = append(args, "--agent-name", opts.AgentName)
	}
	if opts.AgentType != "" {
		args = append(args, "--agent-type", opts.AgentType)
	}
	if opts.Provider != "" {
		args = append(args, "--provider", opts.Provider)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ModelVersion != "" {
		args = append(args, "--model-version", opts.ModelVersion)
	}
	args = append(args, "--json")

	result, err := k.execWrite(context.Background(), args)
	if err != nil {
		return nil, fmt.Errorf("kno lease create: %w", err)
	}
	if result.exitCode != 0 {
		return nil, fmt.Errorf("kno lease create: %s", firstNonEmpty(result.stderr, "failed"))
	}

	var record knoRecord
	if err := json.Unmarshal([]byte(result.stdout), &record); err != nil {
		id := strings.TrimSpace(result.stdout)
		if id == "" {
			return nil, fmt.Errorf("kno lease create: failed to parse response")
		}
		return &LeaseResult{ID: id}, nil
	}

	leaseResult := &LeaseResult{ID: record.ID}
	if record.Lease != nil {
		leaseResult.Lease = record.Lease
	}
	return leaseResult, nil
}

func (k *KnotsBackend) TerminateLease(leaseID string, repoPath string) error {
	result, err := k.execWrite(context.Background(), []string{"lease", "terminate", leaseID})
	if err != nil {
		return fmt.Errorf("kno lease terminate %s: %w", leaseID, err)
	}
	if result.exitCode != 0 {
		return fmt.Errorf("kno lease terminate %s: %s", leaseID, firstNonEmpty(result.stderr, "failed"))
	}
	return nil
}

func (k *KnotsBackend) BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error) {
	return nil, fmt.Errorf("KERNL DISPATCH FAILURE: knots backend buildPollPrompt not yet implemented; use scope refinement worker")
}

func (k *KnotsBackend) Capabilities() BackendCapabilities {
	return FullCapabilities
}

func (k *KnotsBackend) Comment(id string, body string, repoPath string) error {
	return fmt.Errorf("KERNL DISPATCH FAILURE: knots backend does not support comment")
}

func (k *KnotsBackend) resolveRepoPath() string {
	if k.repoPath != "" {
		return k.repoPath
	}
	dir, _ := os.Getwd()
	return dir
}

func (k *KnotsBackend) buildBaseArgs() []string {
	rp := k.resolveRepoPath()
	args := []string{"--repo-root", rp}
	if k.knoDB != "" {
		args = append(args, "--db", k.knoDB)
	} else {
		args = append(args, "--db", filepath.Join(rp, ".knots", "cache", "state.sqlite"))
	}
	return args
}

func (k *KnotsBackend) execRead(ctx context.Context, args []string) (*knoExecResult, error) {
	fullArgs := append(k.buildBaseArgs(), args...)
	return k.runCommand(ctx, fullArgs)
}

func (k *KnotsBackend) execWrite(ctx context.Context, args []string) (*knoExecResult, error) {
	fullArgs := append(k.buildBaseArgs(), args...)
	return k.withWriteSerialization(func() (*knoExecResult, error) {
		return k.runCommandWithRetry(ctx, fullArgs)
	})
}

func (k *KnotsBackend) withWriteSerialization(fn func() (*knoExecResult, error)) (*knoExecResult, error) {
	key := k.resolveRepoPath()
	k.writeQueueMu.Lock()
	q, exists := k.writeQueues[key]
	if !exists {
		q = &knoQueue{tail: make(chan struct{}, 1), pending: 0}
		q.tail <- struct{}{}
		k.writeQueues[key] = q
	}
	q.pending++
	k.writeQueueMu.Unlock()

	defer func() {
		k.writeQueueMu.Lock()
		q.pending--
		if q.pending == 0 {
			delete(k.writeQueues, key)
		}
		k.writeQueueMu.Unlock()
	}()

	<-q.tail
	result, err := fn()
	k.writeQueueMu.Lock()
	if q2, ok := k.writeQueues[key]; ok {
		q2.tail <- struct{}{}
	}
	k.writeQueueMu.Unlock()
	return result, err
}

func (k *KnotsBackend) runCommand(ctx context.Context, fullArgs []string) (*knoExecResult, error) {
	timeout := time.Duration(knoCommandTimeoutMs) * time.Millisecond
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, k.knoBin, fullArgs...)
	cmd.Dir = k.resolveRepoPath()
	cmd.Env = os.Environ()

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	exitCode := 0
	timedOut := false
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		if timeoutCtx.Err() == context.DeadlineExceeded {
			timedOut = true
			timeoutMsg := fmt.Sprintf("knots command timed out after %dms", knoCommandTimeoutMs)
			stderrStr = timeoutMsg + "\n" + stderrStr
		}
	}

	slog.Debug("kno exec completed", "args", fullArgs, "exitCode", exitCode, "timedOut", timedOut)

	return &knoExecResult{
		stdout:   stdoutStr,
		stderr:   stderrStr,
		exitCode: exitCode,
	}, nil
}

func (k *KnotsBackend) runCommandWithRetry(ctx context.Context, fullArgs []string) (*knoExecResult, error) {
	result, err := k.runCommand(ctx, fullArgs)
	if err != nil {
		return result, err
	}
	if result.exitCode == 0 {
		return result, nil
	}

	if !isKnoRetriable(result.stderr) {
		return result, nil
	}

	for _, delay := range knoRetryDelays {
		slog.Debug("kno retrying after retriable error", "delay", delay, "stderr", result.stderr)
		time.Sleep(delay)
		result, err = k.runCommand(ctx, fullArgs)
		if err != nil {
			return result, err
		}
		if result.exitCode == 0 {
			return result, nil
		}
		if !isKnoRetriable(result.stderr) {
			return result, nil
		}
	}

	return result, nil
}

func isKnoRetriable(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, knoRetriableLock) ||
		strings.Contains(lower, knoRetriableBusy) ||
		strings.Contains(lower, knoRetriableTimeout)
}

type knoWorkflowDefinition struct {
	ID             string `json:"id"`
	Description    string `json:"description"`
	InitialState   string `json:"initial_state"`
	States         []string `json:"states"`
	TerminalStates []string `json:"terminal_states"`
	Transitions    []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"transitions"`
}

type knoRecord struct {
	ID          string         `json:"id"`
	Alias       string         `json:"alias,omitempty"`
	Title       string         `json:"title"`
	State       string         `json:"state"`
	ProfileID   string         `json:"profile_id,omitempty"`
	WorkflowID  string         `json:"workflow_id,omitempty"`
	UpdatedAt   string         `json:"updated_at"`
	Body        string         `json:"body,omitempty"`
	Description string         `json:"description,omitempty"`
	Acceptance  string         `json:"acceptance,omitempty"`
	Priority    *int           `json:"priority,omitempty"`
	Type        string         `json:"type,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Notes       json.RawMessage `json:"notes,omitempty"`
	LeaseID     string         `json:"lease_id,omitempty"`
	Lease       *knoLease      `json:"lease,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
}

type knoLease struct {
	LeaseType string        `json:"lease_type"`
	Nickname  string        `json:"nickname"`
	AgentInfo *knoAgentInfo `json:"agent_info,omitempty"`
}

type knoAgentInfo struct {
	AgentType    string `json:"agent_type"`
	Provider     string `json:"provider"`
	AgentName    string `json:"agent_name"`
	Model        string `json:"model"`
	ModelVersion string `json:"model_version"`
}

type knoEdge struct {
	Src  string `json:"src"`
	Kind string `json:"kind"`
	Dst  string `json:"dst"`
}

func knotRecordToBead(rec knoRecord, repoPath string) Bead {
	priority := 2
	if rec.Priority != nil {
		p := *rec.Priority
		if p >= 0 && p <= 4 && p != 0 {
			priority = p
		}
	}

	beatType := rec.Type
	if beatType == "" {
		beatType = "task"
	}

	metadata := make(map[string]any)
	if rec.LeaseID != "" {
		metadata["lease_id"] = rec.LeaseID
	}
	if rec.Lease != nil && rec.Lease.AgentInfo != nil {
		metadata["agent_type"] = rec.Lease.AgentInfo.AgentType
		metadata["provider"] = rec.Lease.AgentInfo.Provider
		metadata["agent_name"] = rec.Lease.AgentInfo.AgentName
		metadata["model"] = rec.Lease.AgentInfo.Model
		metadata["model_version"] = rec.Lease.AgentInfo.ModelVersion
	}
	if rec.ProfileID != "" {
		metadata["knotsProfileId"] = rec.ProfileID
	}
	if rec.WorkflowID != "" {
		metadata["knotsWorkflowEtag"] = rec.WorkflowID
	}

	var labels []string
	if rec.Tags != nil {
		labels = rec.Tags
	} else {
		labels = []string{}
	}

	notes := ""
	if rec.Body != "" {
		notes = rec.Body
	}

	return Bead{
		ID:          rec.ID,
		Type:        beatType,
		State:       rec.State,
		Title:       rec.Title,
		Description: rec.Description,
		Notes:       notes,
		Acceptance:  rec.Acceptance,
		Priority:    priority,
		Labels:      labels,
		Owner:       rec.LeaseID,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
		RepoPath:    repoPath,
		Metadata:    metadata,
		ProfileID:   rec.ProfileID,
	}
}