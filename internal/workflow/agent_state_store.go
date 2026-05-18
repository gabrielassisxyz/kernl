package workflow

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AgentRuntime is the high-frequency runtime state of an agent attached to a bead.
// Spec §4.4 — kept out of bd.description (D1=C) to avoid lost-update under concurrent writes.
type AgentRuntime struct {
	AgentState      AgentState `json:"agent_state,omitempty"`
	AgentSessionID  string     `json:"agent_session_id,omitempty"`
	AgentStartedAt  time.Time  `json:"agent_started_at,omitempty"`
	LastHeartbeatAt time.Time  `json:"last_heartbeat_at,omitempty"`
	FollowUpCount   int        `json:"follow_up_count,omitempty"`
}

// AgentStateStore is a per-bead JSON store with atomic write + in-process mutex.
type AgentStateStore struct {
	dir   string
	locks sync.Map // map[string]*sync.Mutex (key: beadID)
}

// NewAgentStateStore creates the store, ensuring the directory exists.
func NewAgentStateStore(dir string) (*AgentStateStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &AgentStateStore{dir: dir}, nil
}

// Dir returns the store directory path.
func (s *AgentStateStore) Dir() string { return s.dir }

func (s *AgentStateStore) lockFor(id string) *sync.Mutex {
	m, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

func (s *AgentStateStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// Load returns the AgentRuntime for the bead. Missing or corrupted file returns the zero value + WARN.
func (s *AgentStateStore) Load(id string) (AgentRuntime, error) {
	l := s.lockFor(id)
	l.Lock()
	defer l.Unlock()
	data, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return AgentRuntime{}, nil
	}
	if err != nil {
		return AgentRuntime{}, err
	}
	var v AgentRuntime
	if err := json.Unmarshal(data, &v); err != nil {
		log.Printf("WARN agent_state_store: corrupted JSON for %s: %v — recovering with defaults", id, err)
		return AgentRuntime{}, nil
	}
	return v, nil
}

// Save writes the AgentRuntime atomically: tempfile + rename.
func (s *AgentStateStore) Save(id string, v AgentRuntime) error {
	l := s.lockFor(id)
	l.Lock()
	defer l.Unlock()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, id+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, s.path(id))
}

// Purge removes the agent state file for the bead. Idempotent.
func (s *AgentStateStore) Purge(id string) error {
	l := s.lockFor(id)
	l.Lock()
	defer l.Unlock()
	if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
