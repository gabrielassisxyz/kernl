package session

import "sync"

// NudgeRegistry tracks the spawn-context per active session so the API layer
// can rerun an interrupted agent with a manual follow-up prompt without
// having to reconstruct the agent pool, worktree, or captured opencode
// session ID from scratch.
//
// Lifecycle (driven by the driver):
//   - Upsert({BeadID, RepoPath, Cwd, Running=true}) before spawn.
//   - SetOpencodeSessionID(...) when the NDJSON stream emits a ses_xxx id.
//   - SetRunning(false) after proc.Wait() returns.
//
// Nudge readers call Get to snapshot the record before invoking a respawn.
type NudgeRegistry struct {
	mu      sync.RWMutex
	records map[string]*NudgeRecord
}

type NudgeRecord struct {
	BeadID            string
	RepoPath          string
	Cwd               string
	OpencodeSessionID string
	Running           bool
}

func NewNudgeRegistry() *NudgeRegistry {
	return &NudgeRegistry{records: make(map[string]*NudgeRecord)}
}

func (r *NudgeRegistry) Upsert(sessionID string, partial NudgeRecord) {
	if r == nil || sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.records[sessionID]
	if !ok {
		existing = &NudgeRecord{}
		r.records[sessionID] = existing
	}
	if partial.BeadID != "" {
		existing.BeadID = partial.BeadID
	}
	if partial.RepoPath != "" {
		existing.RepoPath = partial.RepoPath
	}
	if partial.Cwd != "" {
		existing.Cwd = partial.Cwd
	}
	if partial.OpencodeSessionID != "" {
		existing.OpencodeSessionID = partial.OpencodeSessionID
	}
	existing.Running = partial.Running
}

func (r *NudgeRegistry) SetRunning(sessionID string, running bool) {
	if r == nil || sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.records[sessionID]; ok {
		rec.Running = running
	}
}

func (r *NudgeRegistry) SetOpencodeSessionID(sessionID, oid string) {
	if r == nil || sessionID == "" || oid == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.records[sessionID]; ok {
		rec.OpencodeSessionID = oid
	}
}

func (r *NudgeRegistry) Get(sessionID string) (NudgeRecord, bool) {
	if r == nil || sessionID == "" {
		return NudgeRecord{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.records[sessionID]
	if !ok {
		return NudgeRecord{}, false
	}
	return *rec, true
}
