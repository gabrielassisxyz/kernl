package app

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// lockTestConfig is deliberately lighter than testConfig: AcquireGraphLock
// reads nothing but the vault root, and a config that also builds a backend
// would make a failure here ambiguous.
func lockTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{}
	cfg.Vault.Root = t.TempDir()
	return cfg
}

func TestAcquireGraphLockSucceedsOnAFreeDatabase(t *testing.T) {
	cfg := lockTestConfig(t)

	release, err := AcquireGraphLock(cfg)
	if err != nil {
		t.Fatalf("AcquireGraphLock on a free database: %v", err)
	}
	t.Cleanup(func() { _ = release() })

	lockPath := filepath.Join(cfg.Vault.Root, graphDBFileName+graphLockSuffix)
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("lock file should hold a pid, got %q: %v", raw, err)
	}
	if pid != os.Getpid() {
		t.Errorf("lock file records pid %d, want this process %d", pid, os.Getpid())
	}
}

func TestAcquireGraphLockReleaseIsIdempotent(t *testing.T) {
	release, err := AcquireGraphLock(lockTestConfig(t))
	if err != nil {
		t.Fatalf("AcquireGraphLock: %v", err)
	}

	if err := release(); err != nil {
		t.Fatalf("first release: %v", err)
	}
	// runServe both defers the release and can reach it on a shutdown path,
	// so a second call must be a no-op rather than an unlock of whatever
	// descriptor number was reused in between.
	if err := release(); err != nil {
		t.Fatalf("second release must be a no-op, got: %v", err)
	}
}

func TestAcquireGraphLockReportsARealErrorRatherThanContention(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses the directory permission this test relies on")
	}
	cfg := lockTestConfig(t)
	if err := os.Chmod(cfg.Vault.Root, 0o555); err != nil {
		t.Fatalf("chmod vault root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfg.Vault.Root, 0o755) })

	_, err := AcquireGraphLock(cfg)
	if err == nil {
		t.Fatal("AcquireGraphLock must fail when it cannot create the lock file")
	}
	var locked GraphLockedError
	if errors.As(err, &locked) {
		t.Fatalf("an unwritable directory is a malfunction, not contention; got %v", err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the dispatch-failure marker, got %q", err)
	}
}

func TestIsLockContentionAcceptsBothErrnosAndNothingElse(t *testing.T) {
	// EWOULDBLOCK and EAGAIN are the same value on Linux and are not
	// guaranteed to be on every Unix; darwin is a release target, so both
	// are named. Anything else must stay a malfunction: telling the operator
	// to go stop a process that does not exist is worse than the raw errno.
	for _, errno := range []syscall.Errno{syscall.EWOULDBLOCK, syscall.EAGAIN} {
		if !isLockContention(errno) {
			t.Errorf("errno %v must count as contention", errno)
		}
	}
	for _, errno := range []syscall.Errno{syscall.EACCES, syscall.ENOSPC, syscall.EIO} {
		if isLockContention(errno) {
			t.Errorf("errno %v is a malfunction, not contention", errno)
		}
	}
}

func TestReadRecordedPIDTreatsUnusableContentAsAbsent(t *testing.T) {
	// Every one of these is reachable in practice: the owner truncates the
	// file AFTER it acquires the lock, so a contender can read it empty or
	// half-written. None may fail the refusal - a missing pid degrades the
	// message, it does not turn a correct refusal into an error.
	for name, content := range map[string]string{
		"empty":      "",
		"whitespace": "  \n",
		"garbage":    "not-a-pid\n",
		"negative":   "-1\n",
		"zero":       "0\n",
		"truncated":  "",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "graph.lock")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("write lock file: %v", err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("open lock file: %v", err)
			}
			defer f.Close()

			if pid := readRecordedPID(f); pid != 0 {
				t.Errorf("content %q should read as no pid, got %d", content, pid)
			}
		})
	}
}

func TestGraphLockedErrorNamesThePIDOnlyWhenItIsKnown(t *testing.T) {
	withPID := newGraphLockedError("/vault/.kernl-graph.db", 4242)
	if !strings.Contains(withPID.Error(), "recorded pid 4242") {
		t.Errorf("a known pid must appear as recorded, got %q", withPID)
	}

	withoutPID := newGraphLockedError("/vault/.kernl-graph.db", 0)
	if strings.Contains(withoutPID.Error(), "pid") {
		t.Errorf("an unknown pid must not be mentioned at all, got %q", withoutPID)
	}

	for _, err := range []GraphLockedError{withPID, withoutPID} {
		if err.Path != "/vault/.kernl-graph.db" {
			t.Errorf("the contested database must always be carried, got %q", err.Path)
		}
		if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Errorf("missing the dispatch-failure marker in %q", err)
		}
		if !strings.Contains(err.Error(), "Fix:") {
			t.Errorf("missing the actionable fix in %q", err)
		}
	}
}
