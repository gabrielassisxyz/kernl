//go:build integration

package app

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// holderRootEnv turns this test binary into the second process. Cross-process
// contention is the only contract flock actually promises: Linux documents
// that two descriptors in ONE process contend, darwin documents locks as
// belonging to the file rather than the descriptor, and darwin is a release
// target. A same-process test would therefore pass here and prove nothing
// about half the platforms this ships to - hence a real second process, and
// hence integration-tagged, since it spawns one.
const holderRootEnv = "KERNL_GRAPH_LOCK_HOLDER_ROOT"

// TestGraphLockHolderProcess is not a test. It is the child: it takes the
// lock, says so, and holds it until its stdin closes.
func TestGraphLockHolderProcess(t *testing.T) {
	root := os.Getenv(holderRootEnv)
	if root == "" {
		t.Skip("helper process, driven by TestAcquireGraphLockRefusesASecondProcess")
	}
	cfg := &config.Config{}
	cfg.Vault.Root = root

	release, err := AcquireGraphLock(cfg)
	if err != nil {
		t.Fatalf("helper could not acquire the lock: %v", err)
	}
	if _, err := os.Stdout.WriteString("ACQUIRED\n"); err != nil {
		t.Fatalf("helper could not report: %v", err)
	}
	_, _ = io.ReadAll(os.Stdin) // held until the parent closes stdin
	_ = release()
}

// startLockHolder spawns the child and returns it once it holds the lock.
func startLockHolder(t *testing.T, root string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestGraphLockHolderProcess", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), holderRootEnv+"="+root)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("holder stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("holder stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})

	acquired := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "ACQUIRED" {
				acquired <- scanner.Text()
				return
			}
		}
		close(acquired)
	}()

	select {
	case _, ok := <-acquired:
		if !ok {
			t.Fatal("holder exited without taking the lock")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("holder never reported taking the lock")
	}
	return cmd
}

func TestAcquireGraphLockRefusesASecondProcess(t *testing.T) {
	root := t.TempDir()
	holder := startLockHolder(t, root)

	cfg := &config.Config{}
	cfg.Vault.Root = root

	_, err := AcquireGraphLock(cfg)
	if err == nil {
		t.Fatal("a second process must be refused while the first holds the lock")
	}

	var locked GraphLockedError
	if !errors.As(err, &locked) {
		t.Fatalf("refusal must be a GraphLockedError, got %T: %v", err, err)
	}
	if locked.PID != holder.Process.Pid {
		t.Errorf("refusal records pid %d, want the holder %d", locked.PID, holder.Process.Pid)
	}
	if !strings.Contains(locked.Path, graphDBFileName) {
		t.Errorf("refusal must name the contested database, got %q", locked.Path)
	}
	if !strings.Contains(err.Error(), "recorded pid") {
		t.Errorf("the message must present the pid as recorded, not as proof of ownership: %q", err)
	}
}

func TestAcquireGraphLockAfterTheOwnerIsKilled(t *testing.T) {
	root := t.TempDir()
	holder := startLockHolder(t, root)

	// SIGKILL, not a clean stop: the whole reason this design needs no lease
	// or heartbeat is that the kernel drops the flock when the process dies,
	// however it dies. A stale lock here would mean the operator has to
	// delete a file by hand after every crash.
	if err := holder.Process.Kill(); err != nil {
		t.Fatalf("kill holder: %v", err)
	}
	_, _ = holder.Process.Wait()

	cfg := &config.Config{}
	cfg.Vault.Root = root

	release, err := AcquireGraphLock(cfg)
	if err != nil {
		t.Fatalf("a killed owner must leave no stale lock, got: %v", err)
	}
	_ = release()
}
