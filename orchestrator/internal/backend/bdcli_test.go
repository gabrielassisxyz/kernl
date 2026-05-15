package backend

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeBdServer struct {
	scriptPath string
	tmpDir     string
	responses  map[string]string
	exitCodes  map[string]int
	stderrs    map[string]string
}

func newFakeBdServer(t *testing.T) *fakeBdServer {
	t.Helper()
	tmpDir := t.TempDir()
	return &fakeBdServer{
		tmpDir:    tmpDir,
		responses: make(map[string]string),
		exitCodes: make(map[string]int),
		stderrs:   make(map[string]string),
	}
}

func (f *fakeBdServer) setResponse(cmd string, stdout string) {
	f.responses[cmd] = stdout
}

func (f *fakeBdServer) setExitCode(cmd string, code int) {
	f.exitCodes[cmd] = code
}

func (f *fakeBdServer) setStderr(cmd string, stderr string) {
	f.stderrs[cmd] = stderr
}

func (f *fakeBdServer) writeScript(t *testing.T) string {
	t.Helper()
	type resp struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exitCode"`
	}

	responses := make(map[string]resp)
	for k, v := range f.responses {
		responses[k] = resp{Stdout: v, Stderr: f.stderrs[k], ExitCode: f.exitCodes[k]}
	}

	data, _ := json.Marshal(responses)

	script := filepath.Join(f.tmpDir, "fake-bd.sh")
	scriptContent := "#!/bin/sh\n" +
		"RESPONSES='" + string(data) + "'\n" +
		"CMD=\"$1\"\n" +
		"shift\n" +
		"case \"$CMD\" in\n" +
		"  list) KEY=\"list\";;\n" +
		"  show) KEY=\"show\";;\n" +
		"  create) KEY=\"create\";;\n" +
		"  update) KEY=\"update\";;\n" +
		"  close) KEY=\"close\";;\n" +
		"  delete) KEY=\"delete\";;\n" +
		"  search) KEY=\"search\";;\n" +
		"  query) KEY=\"query\";;\n" +
		"  list-workflows) KEY=\"list-workflows\";;\n" +
		"  sync) KEY=\"sync\";;\n" +
		"  add-dep) KEY=\"add-dep\";;\n" +
		"  remove-dep) KEY=\"remove-dep\";;\n" +
		"  list-deps) KEY=\"list-deps\";;\n" +
		"  *) KEY=\"$CMD\";;\n" +
		"esac\n" +
		"STDOUT=$(echo \"$RESPONSES\" | python3 -c \"import sys,json; d=json.load(sys.stdin); print(d.get('$KEY',{}).get('stdout',''))\" 2>/dev/null || echo '')\n" +
		"STDERR=$(echo \"$RESPONSES\" | python3 -c \"import sys,json; d=json.load(sys.stdin); print(d.get('$KEY',{}).get('stderr',''))\" 2>/dev/null || echo '')\n" +
		"EXITCODE=$(echo \"$RESPONSES\" | python3 -c \"import sys,json; d=json.load(sys.stdin); print(d.get('$KEY',{}).get('exitCode','0'))\" 2>/dev/null || echo 0)\n" +
		"echo \"$STDOUT\"\n" +
		"echo \"$STDERR\" >&2\n" +
		"exit $EXITCODE\n"
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("writing fake bd script: %v", err)
	}
	f.scriptPath = script
	return script
}

func newTestBdCliBackend(t *testing.T) *BdCliBackend {
	t.Helper()
	tmpDir := t.TempDir()
	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    "echo",
		bdDB:     "",
	}
	return b
}

func TestIsReadOnlyCommand(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"list", "--repo", "."}, true},
		{[]string{"show", "1"}, true},
		{[]string{"search", "test"}, true},
		{[]string{"query", "type:task"}, true},
		{[]string{"ready"}, true},
		{[]string{"dep", "list", "1"}, true},
		{[]string{"create", "task"}, false},
		{[]string{"update", "1"}, false},
		{[]string{"dep", "add"}, false},
		{[]string{}, false},
	}
	for _, tt := range tests {
		got := IsReadOnlyCommand(tt.args)
		if got != tt.want {
			t.Errorf("IsReadOnlyCommand(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestIsIdempotentWriteCommand(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"update", "1", "--state", "active"}, true},
		{[]string{"label", "add", "1", "stage:impl"}, true},
		{[]string{"label", "remove", "1", "stage:impl"}, true},
		{[]string{"sync", "--import-only"}, true},
		{[]string{"dep", "remove", "1", "2"}, true},
		{[]string{"create", "task"}, false},
		{[]string{"close", "1"}, false},
		{[]string{"dep", "add", "1", "2"}, false},
		{[]string{"list"}, false},
	}
	for _, tt := range tests {
		got := isIdempotentWriteCommand(tt.args)
		if got != tt.want {
			t.Errorf("isIdempotentWriteCommand(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestCanRetryAfterTimeout(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"list"}, true},
		{[]string{"show", "1"}, true},
		{[]string{"update", "1"}, true},
		{[]string{"sync"}, true},
		{[]string{"create", "task"}, false},
		{[]string{"close", "1"}, false},
	}
	for _, tt := range tests {
		got := canRetryAfterTimeout(tt.args)
		if got != tt.want {
			t.Errorf("canRetryAfterTimeout(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestShouldUseNoDBByDefault(t *testing.T) {
	origBD := os.Getenv(bdNoDBEnv)
	origRead := os.Getenv("KERNL_BD_READ_NO_DB")
	defer func() {
		os.Setenv(bdNoDBEnv, origBD)
		os.Setenv("KERNL_BD_READ_NO_DB", origRead)
	}()

	os.Unsetenv(bdNoDBEnv)
	os.Unsetenv("KERNL_BD_READ_NO_DB")

	if !shouldUseNoDBByDefault([]string{"list"}) {
		t.Error("shouldUseNoDBByDefault(list) = false, want true for read command")
	}
	if shouldUseNoDBByDefault([]string{"create"}) {
		t.Error("shouldUseNoDBByDefault(create) = true, want false for write command")
	}

	os.Setenv("KERNL_BD_READ_NO_DB", "0")
	if shouldUseNoDBByDefault([]string{"list"}) {
		t.Error("shouldUseNoDBByDefault(list) with KERNL_BD_READ_NO_DB=0 should be false")
	}

	os.Unsetenv("KERNL_BD_READ_NO_DB")
	os.Setenv(bdNoDBEnv, "true")
	if !shouldUseNoDBByDefault([]string{"create"}) {
		t.Error("shouldUseNoDBByDefault(create) with BD_NO_DB=true should be true")
	}
}

func TestIsEmbeddedDoltPanic(t *testing.T) {
	tests := []struct {
		result  *ExecResult
		want    bool
	}{
		{&ExecResult{Stderr: "panic: runtime error: invalid memory address or nil pointer dereference", ExitCode: 1}, true},
		{&ExecResult{Stdout: "SetCrashOnFatalError", ExitCode: 1}, true},
		{&ExecResult{Stderr: "some other error", ExitCode: 1}, false},
		{&ExecResult{ExitCode: 0}, false},
	}
	for _, tt := range tests {
		got := isEmbeddedDoltPanic(tt.result)
		if got != tt.want {
			t.Errorf("isEmbeddedDoltPanic(%+v) = %v, want %v", tt.result, got, tt.want)
		}
	}
}

func TestIsOutOfSyncError(t *testing.T) {
	result := &ExecResult{Stderr: "Error: Database out of sync with JSONL", ExitCode: 1}
	if !isOutOfSyncError(result) {
		t.Error("isOutOfSyncError should detect out-of-sync error")
	}

	result2 := &ExecResult{Stderr: "some other error", ExitCode: 1}
	if isOutOfSyncError(result2) {
		t.Error("isOutOfSyncError should not detect non-sync errors")
	}
}

func TestBaseArgs(t *testing.T) {
	tests := []struct {
		bdDB string
		args []string
		want []string
	}{
		{"", []string{"list"}, []string{"list"}},
		{"/tmp/db", []string{"list"}, []string{"--db", "/tmp/db", "list"}},
	}
	for _, tt := range tests {
		got := baseArgs(tt.bdDB, tt.args)
		if len(got) != len(tt.want) {
			t.Errorf("baseArgs(%q, %v) = %v, want %v", tt.bdDB, tt.args, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("baseArgs(%q, %v)[%d] = %q, want %q", tt.bdDB, tt.args, i, got[i], tt.want[i])
			}
		}
	}
}

func TestContainsNoDaemonFlag(t *testing.T) {
	if !containsNoDaemonFlag([]string{"sync", "--no-daemon"}) {
		t.Error("should find --no-daemon flag")
	}
	if containsNoDaemonFlag([]string{"sync", "--import-only"}) {
		t.Error("should not find --no-daemon flag")
	}
}

func TestStripNoDaemonFlag(t *testing.T) {
	result := stripNoDaemonFlag([]string{"sync", "--import-only", "--no-daemon"})
	expected := []string{"sync", "--import-only"}
	if len(result) != len(expected) {
		t.Fatalf("stripNoDaemonFlag: got %v, want %v", result, expected)
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("stripNoDaemonFlag[%d]: got %q, want %q", i, result[i], expected[i])
		}
	}
}

func TestIsUnknownNoDaemonFlagError(t *testing.T) {
	result := &ExecResult{Stderr: "Error: unknown flag: --no-daemon", ExitCode: 1}
	if !isUnknownNoDaemonFlagError(result) {
		t.Error("isUnknownNoDaemonFlagError should detect unknown flag error")
	}

	result2 := &ExecResult{Stderr: "some other error", ExitCode: 1}
	if isUnknownNoDaemonFlagError(result2) {
		t.Error("isUnknownNoDaemonFlagError should not detect non-flag errors")
	}
}

func TestLockOwnerReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	lockDir := filepath.Join(tmpDir, "test-lock")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(lockDir, "owner.json")

	owner := readLockOwner(lockFile)
	if owner != nil {
		t.Error("readLockOwner on nonexistent file should return nil")
	}

	testOwner := lockOwner{
		PID:        os.Getpid(),
		RepoPath:   "/test/repo",
		AcquiredAt: time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(testOwner)
	if err := os.WriteFile(lockFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	readBack := readLockOwner(lockFile)
	if readBack == nil {
		t.Fatal("readLockOwner should return owner from valid file")
	}
	if readBack.PID != testOwner.PID {
		t.Errorf("PID: got %d, want %d", readBack.PID, testOwner.PID)
	}
	if readBack.RepoPath != testOwner.RepoPath {
		t.Errorf("RepoPath: got %q, want %q", readBack.RepoPath, testOwner.RepoPath)
	}
}

func TestAcquireAndReleaseRepoProcessLock(t *testing.T) {
	tmpDir := t.TempDir()
	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    "bd",
	}

	ctx := context.Background()
	release, err := b.acquireRepoProcessLock(ctx, tmpDir)
	if err != nil {
		t.Fatalf("acquireRepoProcessLock: %v", err)
	}

	lockDir := filepath.Join(b.locksDir, fmt.Sprintf("%x", sha1.Sum([]byte(tmpDir))))
	if _, err := os.Stat(lockDir); os.IsNotExist(err) {
		t.Error("lock directory should exist after acquisition")
	}

	release()

	time.Sleep(50 * time.Millisecond)

	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after release")
	}
}

func TestWithRepoSerialization(t *testing.T) {
	tmpDir := t.TempDir()
	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    "bd",
	}

	ctx := context.Background()
	executed := false
	result, err := b.withRepoSerialization(ctx, tmpDir, func() (*ExecResult, error) {
		executed = true
		return &ExecResult{ExitCode: 0, Stdout: "ok"}, nil
	})

	if err != nil {
		t.Fatalf("withRepoSerialization: %v", err)
	}
	if !executed {
		t.Error("withRepoSerialization did not execute function")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", result.ExitCode)
	}
}

func TestParseNDJSONBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{
			name:    "single object",
			input:   `{"id": "1", "title": "test"}`,
			wantLen: 1,
		},
		{
			name:    "multiple objects",
			input:   `{"id": "1"}` + "\n" + `{"id": "2"}`,
			wantLen: 2,
		},
		{
			name:    "empty lines skipped",
			input:   `{"id": "1"}` + "\n\n" + `{"id": "2"}`,
			wantLen: 2,
		},
		{
			name:    "non-JSON lines skipped",
			input:   `{"id": "1"}` + "\n" + "not json" + "\n" + `{"id": "2"}`,
			wantLen: 2,
		},
		{
			name:    "empty input",
			input:   "",
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseNDJSONBytes([]byte(tt.input))
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.input == "" {
				return
			}
			if result == nil {
				t.Fatal("result is nil")
			}
			var arr []json.RawMessage
			if err := json.Unmarshal(result, &arr); err == nil {
				if len(arr) != tt.wantLen {
					t.Errorf("got %d items, want %d", len(arr), tt.wantLen)
				}
			} else {
				if tt.wantLen != 1 {
					t.Errorf("expected %d items but got single object or non-array", tt.wantLen)
				}
			}
		})
	}
}

func TestExecOnceSetsNoDB(t *testing.T) {
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "bd-check-nodb.sh")
	scriptContent := "#!/bin/sh\nif [ \"$BD_NO_DB\" = \"true\" ]; then echo '{\"id\":\"1\"}'; else echo 'no-db not set' >&2; exit 1; fi\n"
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &ExecOptions{ForceNoDB: true}
	result, err := execOnce(ctx, script, "", []string{"list"}, opts)
	if err != nil {
		t.Fatalf("execOnce: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d, stderr: %s", result.ExitCode, result.Stderr)
	}
}

func TestCreateFallsBackToRawOutput(t *testing.T) {
	tmpDir := t.TempDir()

	script := filepath.Join(tmpDir, "bd-create.sh")
	scriptContent := "#!/bin/sh\necho 'proj.42'\n"
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    script,
	}

	beat, err := b.Create(CreateBeatInput{Title: "Test Task"}, tmpDir)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if beat.ID != "proj.42" {
		t.Errorf("expected ID proj.42, got %q", beat.ID)
	}
}

func TestBDPath(t *testing.T) {
	b := &BdCliBackend{
		repoPath: "/test/repo",
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(os.TempDir(), "kernl-bd-locks-test"),
		bdBin:    "custom-bd",
	}
	if b.bdBin != "custom-bd" {
		t.Errorf("bdBin: got %q, want %q", b.bdBin, "custom-bd")
	}
}

func TestWithRepoSerializationParallel(t *testing.T) {
	tmpDir := t.TempDir()
	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    "bd",
	}

	ctx := context.Background()
	execCount := 0
	done := make(chan struct{})

	go func() {
		defer close(done)
		_, err := b.withRepoSerialization(ctx, tmpDir, func() (*ExecResult, error) {
			execCount++
			return &ExecResult{ExitCode: 0, Stdout: "ok"}, nil
		})
		if err != nil {
			t.Errorf("withRepoSerialization: %v", err)
		}
	}()

	_, err := b.withRepoSerialization(ctx, tmpDir, func() (*ExecResult, error) {
		execCount++
		return &ExecResult{ExitCode: 0, Stdout: "ok"}, nil
	})
	if err != nil {
		t.Errorf("withRepoSerialization: %v", err)
	}

	<-done

	if execCount != 2 {
		t.Errorf("expected 2 executions, got %d", execCount)
	}
}

func TestIsPidAlive(t *testing.T) {
	if isPidAlive(0) {
		t.Error("isPidAlive(0) should be false")
	}
	if isPidAlive(-1) {
		t.Error("isPidAlive(-1) should be false")
	}

	currentPid := os.Getpid()
	alive := isPidAlive(currentPid)
	if !alive {
		t.Errorf("isPidAlive(%d) should be true for current process", currentPid)
	}

	nonexistentPid := 4000000
	if isPidAlive(nonexistentPid) {
		t.Errorf("isPidAlive(%d) should be false for nonexistent process", nonexistentPid)
	}
}

func TestIsTimeoutResult(t *testing.T) {
	if !isTimeoutResult(&ExecResult{TimedOut: true}) {
		t.Error("timedOut=true should be timeout")
	}
	if !isTimeoutResult(&ExecResult{Stderr: "bd command timed out after 5000ms"}) {
		t.Error("timeout message should be detected")
	}
	if isTimeoutResult(&ExecResult{ExitCode: 1, Stderr: "some error"}) {
		t.Error("non-timeout error should not be detected")
	}
}

func TestBdCliBackendDeleteIncludesForce(t *testing.T) {
	tmpDir := t.TempDir()

	scriptContent := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"--force\" ]; then\n" +
		"    echo '{\"id\":\"1\"}'\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"echo 'missing --force' >&2\n" +
		"exit 1\n"

	script := filepath.Join(tmpDir, "bd-force.sh")
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    script,
		bdDB:     "",
	}

	err := b.Delete("1", tmpDir)
	if err != nil {
		t.Errorf("Delete with --force: %v", err)
	}
}

func TestBdCliBackendCloseWithEmptyReason(t *testing.T) {
	tmpDir := t.TempDir()

	scriptContent := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"--reason\" ]; then\n" +
		"    echo 'HAS_REASON'\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"echo '{\"state\":\"shipped\"}'\n" +
		"exit 0\n"

	script := filepath.Join(tmpDir, "bd-close.sh")
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    script,
	}

	_, err := b.Close("1", "", tmpDir)
	if err != nil {
		t.Errorf("Close with empty reason should succeed: %v", err)
	}
}

func TestBdCliBackendCloseOmitsReasonWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	scriptContent := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"--reason\" ]; then\n" +
		"    echo 'HAS_REASON' >&2\n" +
		"    exit 1\n" +
		"  fi\n" +
		"done\n" +
		"echo '{\"state\":\"shipped\"}'\n" +
		"exit 0\n"

	script := filepath.Join(tmpDir, "bd-close-no-reason.sh")
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	b := &BdCliBackend{
		repoPath: tmpDir,
		queues:   make(map[string]*repoQueue),
		locksDir: filepath.Join(tmpDir, "locks"),
		bdBin:    script,
	}

	result, err := b.Close("1", "", tmpDir)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if result.State != "shipped" {
		t.Errorf("Close result state: got %q, want %q", result.State, "shipped")
	}
}