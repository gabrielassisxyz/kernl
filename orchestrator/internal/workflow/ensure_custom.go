package workflow

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// BdRunner is the minimal surface needed by EnsureCustomStatuses.
// In production, the BdCliBackend implements it via `bd config get/set status.custom`.
type BdRunner interface {
	GetCustomStatuses() ([]string, error)
	SetCustomStatuses(list []string) error
}

var (
	ensureCacheMu sync.Mutex
	ensureCache   = map[string]bool{}
)

// EnsureCustomStatuses registers the kernl custom statuses with bd idempotently.
// Cache + sentinel pattern mirrors gastown beads_types.go:187 (D7=B / TT3=B).
// Foreign customs already registered are preserved (merge, not overwrite).
func EnsureCustomStatuses(beadsDir string, r BdRunner) error {
	abs, err := filepath.Abs(beadsDir)
	if err != nil {
		return err
	}
	ensureCacheMu.Lock()
	cached := ensureCache[abs]
	ensureCacheMu.Unlock()
	if cached {
		return nil
	}

	current, err := r.GetCustomStatuses()
	if err != nil {
		return err
	}

	need := false
	have := map[string]bool{}
	for _, s := range current {
		have[s] = true
	}
	for _, s := range KernlCustomStatuses {
		if !have[s] {
			need = true
		}
	}

	if need {
		merged := append([]string(nil), current...)
		for _, s := range KernlCustomStatuses {
			if !have[s] {
				merged = append(merged, s)
			}
		}
		sort.Strings(merged)
		if err := r.SetCustomStatuses(merged); err != nil {
			return err
		}
	}

	sentinelPath := filepath.Join(beadsDir, ".kernl-custom-statuses-installed")
	_ = os.WriteFile(sentinelPath, []byte("ok\n"), 0o644)

	ensureCacheMu.Lock()
	ensureCache[abs] = true
	ensureCacheMu.Unlock()
	return nil
}

// ResetEnsureCache is a test hook; clears the in-memory cache.
func ResetEnsureCache() {
	ensureCacheMu.Lock()
	defer ensureCacheMu.Unlock()
	ensureCache = map[string]bool{}
}
