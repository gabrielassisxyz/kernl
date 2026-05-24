package subprocess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

var (
	ErrOutputTooLarge = errors.New("subprocess stdout exceeded limit")
	ErrTimeout        = errors.New("subprocess timeout exceeded")
	ErrNonZeroExit    = errors.New("subprocess exited non-zero")
	ErrParseError     = errors.New("subprocess stdout JSON parsing failed")
)

type FailureCause string

const (
	CauseNonZeroExit    FailureCause = "non-zero"
	CauseTimeout        FailureCause = "timeout"
	CauseParseError     FailureCause = "parse-error"
	CauseOutputTooLarge FailureCause = "output-too-large"
)

// SubprocessError represents a failure that occurred while running an escape hatch subprocess.
type SubprocessError struct {
	Cause  FailureCause
	Stderr string
	Err    error
}

func (e *SubprocessError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("subprocess error (%s): %v. stderr: %s", e.Cause, e.Err, e.Stderr)
	}
	return fmt.Sprintf("subprocess error (%s). stderr: %s", e.Cause, e.Stderr)
}

func (e *SubprocessError) Unwrap() error {
	return e.Err
}

type boundedWriter struct {
	buf       bytes.Buffer
	cap       int
	truncated bool
}

func (w *boundedWriter) Write(p []byte) (n int, err error) {
	if w.truncated {
		return len(p), nil
	}
	spaceLeft := w.cap - w.buf.Len()
	if len(p) > spaceLeft {
		w.buf.Write(p[:spaceLeft])
		w.buf.WriteString("\n... (truncated at 65536 bytes)\n")
		w.truncated = true
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *boundedWriter) String() string {
	return w.buf.String()
}

func (w *boundedWriter) Bytes() []byte {
	return w.buf.Bytes()
}

// RunSubprocessStage runs the command specified in the StageContract's Subprocess field
// in the requested bead worktree directory. It writes the HandoffRequest as JSON to the command's STDIN,
// captures and caps the STDOUT/STDERR separately at 64KB (truncating with a marker if exceeded),
// enforces the spec's timeout (or a default of 5 minutes), and returns the parsed HandoffResponse.
func RunSubprocessStage(ctx context.Context, spec backend.StageContract, req HandoffRequest) (HandoffResponse, error) {
	if spec.Subprocess == nil || len(spec.Subprocess.Command) == 0 {
		return HandoffResponse{}, fmt.Errorf("no subprocess command specified in stage contract")
	}

	// Enforce timeout: read optional timeout field, fallback to 5 minutes if absent/zero/invalid
	timeout := 5 * time.Minute
	if spec.Subprocess.Timeout != "" {
		d, err := time.ParseDuration(spec.Subprocess.Timeout)
		if err == nil && d > 0 {
			timeout = d
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmdName := spec.Subprocess.Command[0]
	var cmdArgs []string
	if len(spec.Subprocess.Command) > 1 {
		cmdArgs = spec.Subprocess.Command[1:]
	}

	cmd := exec.CommandContext(timeoutCtx, cmdName, cmdArgs...)
	cmd.Dir = req.WorktreePath

	// Marshal request JSON to STDIN
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return HandoffResponse{}, fmt.Errorf("failed to marshal handoff request: %w", err)
	}
	cmd.Stdin = bytes.NewReader(reqBytes)

	// Capture STDOUT and STDERR separately (cap at 64KB)
	stdoutWriter := &boundedWriter{cap: 65536}
	stderrWriter := &boundedWriter{cap: 65536}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	err = cmd.Run()

	// Check if STDOUT was truncated
	if stdoutWriter.truncated {
		return HandoffResponse{}, &SubprocessError{
			Cause:  CauseOutputTooLarge,
			Stderr: stderrWriter.String(),
			Err:    ErrOutputTooLarge,
		}
	}

	// Check if we hit the timeout
	if timeoutCtx.Err() != nil {
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return HandoffResponse{}, &SubprocessError{
				Cause:  CauseTimeout,
				Stderr: stderrWriter.String(),
				Err:    ErrTimeout,
			}
		}
		return HandoffResponse{}, timeoutCtx.Err()
	}

	// Check command exit status
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return HandoffResponse{}, &SubprocessError{
				Cause:  CauseNonZeroExit,
				Stderr: stderrWriter.String(),
				Err:    fmt.Errorf("%w: exit status %d", ErrNonZeroExit, exitErr.ExitCode()),
			}
		}
		return HandoffResponse{}, &SubprocessError{
			Cause:  CauseNonZeroExit,
			Stderr: stderrWriter.String(),
			Err:    err,
		}
	}

	// Handle exit 0
	trimmed := bytes.TrimSpace(stdoutWriter.Bytes())
	if len(trimmed) == 0 {
		return HandoffResponse{ContextPayload: ""}, nil
	}

	var resp HandoffResponse
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return HandoffResponse{}, &SubprocessError{
			Cause:  CauseParseError,
			Stderr: stderrWriter.String(),
			Err:    fmt.Errorf("%w: %v", ErrParseError, err),
		}
	}

	return resp, nil
}
