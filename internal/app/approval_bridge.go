package app

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/approvals"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// piApprovalExtension is the tool_call hook pi loads with -e. It is embedded
// rather than shipped as a separate file for the same reason the bridge is a
// kernl subcommand: one binary, nothing to install alongside it.
//
//go:embed assets/pi-approval-gate.ts
var piApprovalExtension string

// EnvApprovalBridgeBin tells the pi extension which binary to ask. claude gets
// the same path on its command line instead, inside the MCP config.
const EnvApprovalBridgeBin = "KERNL_APPROVAL_BRIDGE_BIN"

// ApprovalBridgeInput is what a dispatch knows about the run being gated.
type ApprovalBridgeInput struct {
	StateDir  string
	SessionID string
	BeadID    string
	RepoPath  string
	AgentName string
	Timeout   time.Duration
}

// applyApprovalBridge arms the judgment gate for one dispatch.
//
// It is a no-op unless the operator asked for it (approvalMode: prompt), which
// keeps the default path exactly as it was: an agent that runs unattended.
// When it is asked for, it must either arm the gate completely or fail - an
// agent dispatched under a gate that was only half-wired runs every tool
// unasked while the operator believes otherwise, which is worse than either
// honest alternative.
func applyApprovalBridge(agent *adapter.AgentTarget, env map[string]string, in ApprovalBridgeInput) error {
	if adapter.ShouldBypassPermissions(*agent) {
		return nil
	}

	dialect := adapter.ResolveDialect(agent.Command)
	if !adapter.SupportsApprovalPrompt(dialect) {
		return fmt.Errorf("KERNL DISPATCH FAILURE: agent %q (%s) is configured with approvalMode: prompt, but kernl has no approval bridge for the %s dialect, so it would run every tool unasked - Fix: set approvalMode: auto for this agent, or dispatch it with claude or pi",
			in.AgentName, agent.Command, dialect)
	}
	if in.StateDir == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for bead %s, so kernl has nowhere to park an approval - Fix: set StateDir (app.DefaultStateDir() outside tests)", in.BeadID)
	}

	bridgeBin, err := approvalBridgeBinary()
	if err != nil {
		return err
	}
	agent.ApprovalBridgePath = bridgeBin

	if dialect == adapter.DialectPi {
		extensionPath, err := writePiApprovalExtension(in.StateDir)
		if err != nil {
			return err
		}
		agent.ApprovalExtensionPath = extensionPath
		env[EnvApprovalBridgeBin] = bridgeBin
	}

	for k, v := range adapter.ApprovalBridgeEnvVars(adapter.ApprovalBridgeContext{
		SessionID: in.SessionID,
		StoreDir:  ApprovalStoreDir(in.StateDir),
		BeadID:    in.BeadID,
		RepoPath:  in.RepoPath,
		AgentName: in.AgentName,
		Timeout:   in.Timeout,
	}) {
		env[k] = v
	}

	slog.Info("[dispatch] approval.gate_armed",
		"bead", in.BeadID, "agent", in.AgentName, "dialect", string(dialect), "timeout", in.Timeout.String())
	return nil
}

// ApprovalStoreDir is the one spelling of where gates are parked. The bridge,
// the API and the CLI all derive it from the state directory, and a second
// spelling would have one of them watching a directory nobody writes to.
func ApprovalStoreDir(stateDir string) string {
	return filepath.Join(stateDir, "approvals")
}

// approvalBridgeBinary resolves the running kernl binary by path rather than by
// name. The process that spawns the bridge is an agent CLI, not a login shell,
// so its PATH is whatever kernl inherited - naming the binary would make the
// gate depend on an environment kernl does not control.
func approvalBridgeBinary() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve the kernl binary to use as the approval bridge: %w - Fix: dispatch from an installed kernl rather than a deleted or replaced binary", err)
	}
	return path, nil
}

func writePiApprovalExtension(stateDir string) (string, error) {
	dir := filepath.Join(stateDir, "adapters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating %s for the pi approval extension: %w", dir, err)
	}
	path := filepath.Join(dir, "pi-approval-gate.ts")
	// Rewritten on every dispatch so an upgraded kernl never runs against an
	// extension left behind by an older one.
	if err := os.WriteFile(path, []byte(piApprovalExtension), 0o644); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing the pi approval extension to %s: %w", path, err)
	}
	return path, nil
}

// ApprovalTimeout reads the operator's deadline for a judgment gate, falling
// back to the measured default. A malformed value is an error rather than a
// silent fallback: an operator who wrote "30" meaning minutes must find out
// now, not when a gate expires in 30 nanoseconds.
func ApprovalTimeout(cfg *config.Config) (time.Duration, error) {
	if cfg == nil || cfg.Orchestrator.ApprovalTimeout == "" {
		return approvals.DefaultTimeout, nil
	}
	parsed, err := time.ParseDuration(cfg.Orchestrator.ApprovalTimeout)
	if err != nil {
		return 0, fmt.Errorf("KERNL DISPATCH FAILURE: orchestrator.approvalTimeout is %q, which is not a duration - Fix: use a Go duration such as 30m or 2h", cfg.Orchestrator.ApprovalTimeout)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("KERNL DISPATCH FAILURE: orchestrator.approvalTimeout is %q - a gate with no deadline holds an agent process forever - Fix: use a positive duration such as 30m", cfg.Orchestrator.ApprovalTimeout)
	}
	return parsed, nil
}
