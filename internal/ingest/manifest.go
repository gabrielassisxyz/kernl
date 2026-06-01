package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileState struct {
	Hash        string    `json:"hash"`
	ProcessedAt time.Time `json:"processed_at"`
}

type ManifestManager struct {
	mu           sync.RWMutex
	manifestPath string
	states       map[string]FileState
}

func NewManifestManager(vaultRoot string) *ManifestManager {
	return &ManifestManager{
		manifestPath: filepath.Join(vaultRoot, "vault-llm", "manifest.json"),
		states:       make(map[string]FileState),
	}
}

func (m *ManifestManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			m.states = make(map[string]FileState)
			return nil
		}
		return fmt.Errorf("read manifest: %w", err)
	}

	if err := json.Unmarshal(data, &m.states); err != nil {
		return fmt.Errorf("unmarshal manifest: %w", err)
	}
	return nil
}

func (m *ManifestManager) saveLocked() error {
	dir := filepath.Dir(m.manifestPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir vault-llm: %w", err)
	}

	data, err := json.MarshalIndent(m.states, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(m.manifestPath, data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func (m *ManifestManager) NeedsProcessing(filePath string, content []byte) bool {
	hash := calculateHash(content)

	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.states[filePath]
	if !ok {
		return true
	}
	return state.Hash != hash
}

func (m *ManifestManager) MarkProcessed(filePath string, content []byte) error {
	hash := calculateHash(content)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.states[filePath] = FileState{
		Hash:        hash,
		ProcessedAt: time.Now(),
	}

	return m.saveLocked()
}

func calculateHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
