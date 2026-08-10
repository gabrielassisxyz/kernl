package app

import (
	"context"
	"sync"
	"testing"
)

// setRunBeadPhaseRecorder installs a recorder for the duration of one test and
// returns a reader for what it captured.
func setRunBeadPhaseRecorder(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var seen []string

	runBeadPhaseMu.Lock()
	runBeadPhaseHook = func(phase string) {
		mu.Lock()
		seen = append(seen, phase)
		mu.Unlock()
	}
	runBeadPhaseMu.Unlock()

	t.Cleanup(func() {
		runBeadPhaseMu.Lock()
		runBeadPhaseHook = nil
		runBeadPhaseMu.Unlock()
	})

	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

// TestRunBeadRegistersTurnEndedBeforeStartingReaders pins the ordering that a
// stress loop cannot pin. RunBead used to call r.Start - which launches the
// stdout/stderr reader goroutines - before the session pump registered
// SetOnTurnEnded, leaving a window in which a reader could consume the
// terminal event with a nil callback and silently drop the follow-up signal.
// Whether that window is ever hit depends on the scheduler, so this asserts
// the order directly instead of racing it.
func TestRunBeadRegistersTurnEndedBeforeStartingReaders(t *testing.T) {
	phases := setRunBeadPhaseRecorder(t)

	be := &fakeBackend{state: map[string]string{"kb-1": "implementation"}}
	spawn := &fakeSpawner{script: "{\"type\":\"result\",\"is_error\":true,\"result\":\"error_max_turns\"}\n"}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM(), LogDir: t.TempDir()})

	_, _ = d.RunBead(context.Background(), RunBeadInput{
		BeadID: "kb-1", RepoPath: t.TempDir(), Command: "claude", AgentName: "claude",
	})

	got := phases()
	if len(got) != 2 {
		t.Fatalf("expected both phases to be reported, got %v", got)
	}
	if got[0] != phasePumpRegistered || got[1] != phaseReadersStarted {
		t.Errorf("turn-ended callback must be registered before any reader can run, got order %v", got)
	}
}
