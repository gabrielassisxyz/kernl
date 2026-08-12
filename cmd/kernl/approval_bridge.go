package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/approvals"
)

// bridgeAdapters are the agents whose permission mechanism has been measured
// against a real CLI. Adding a name here without measuring it produces a gate
// that never fires, which is indistinguishable from an agent that never asks.
var bridgeAdapters = []string{"claude", "pi"}

// runApprovalBridge is the process an agent talks to when it needs permission.
//
// It is invoked by the agent, never by a person: claude spawns it as an MCP
// server (--mcp), and pi's extension spawns it once per gated tool call. Both
// modes read from stdin and block until the request is answered, denied, or
// expires - the blocking IS the feature, and a bridge that returns early is a
// gate that does not gate.
func runApprovalBridge(v verbContext, args []string) error {
	adapterName, _, rest, err := takeFlag("approval bridge", args, "--adapter")
	if err != nil {
		return err
	}
	mcpMode, rest := parseBoolFlag(rest, "--mcp")
	if err := rejectUnknownFlags("approval bridge", rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: approval bridge takes no positional arguments, got %q - run: kernl approval bridge --help", rest[0])
	}
	if err := checkBridgeAdapter(adapterName); err != nil {
		return err
	}

	store, rc, err := bridgeStoreFromEnv()
	if err != nil {
		return err
	}

	if mcpMode {
		return approvals.ServeMCP(context.Background(), store, adapterName, rc, v.stdin(), v.stdout())
	}
	return askOnce(context.Background(), store, adapterName, rc, v.stdin(), v.stdout())
}

func checkBridgeAdapter(name string) error {
	if name == "" {
		return usagef("KERNL DISPATCH FAILURE: approval bridge requires --adapter - valid: %s. Run: kernl approval bridge --help", strings.Join(bridgeAdapters, ", "))
	}
	for _, a := range bridgeAdapters {
		if a == name {
			return nil
		}
	}
	return usagef("KERNL DISPATCH FAILURE: no approval bridge for adapter %q%s - valid: %s. Run: kernl approval bridge --help",
		name, didYouMean(name, bridgeAdapters), strings.Join(bridgeAdapters, ", "))
}

// askOnce is the one-shot mode: one permission payload in on stdin, one verdict
// out on stdout. It is what the pi extension shells out to, because pi gates a
// tool call from inside a JavaScript hook rather than over a protocol.
func askOnce(ctx context.Context, store *approvals.Store, adapterName string, rc approvals.RequestContext, in io.Reader, out io.Writer) error {
	payload, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: reading the approval payload from stdin: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: the approval payload on stdin is not a JSON object: %w", err)
	}

	outcome, gateErr := approvals.Gate(ctx, store, adapterName, raw, rc)
	if gateErr != nil {
		// The verdict is data, not an exit status: the extension must be able
		// to tell the agent WHY, and a failed gate is a denial, never a pass.
		return emitVerdict(out, approvals.GateOutcome{
			Allowed: false,
			Reason:  "kernl could not raise an approval gate for this call: " + gateErr.Error(),
		})
	}
	return emitVerdict(out, outcome)
}

func emitVerdict(out io.Writer, outcome approvals.GateOutcome) error {
	return json.NewEncoder(out).Encode(map[string]any{
		"allowed":    outcome.Allowed,
		"reason":     outcome.Reason,
		"approvalId": outcome.ApprovalID,
		"remembered": outcome.Remembered,
	})
}

// bridgeStoreFromEnv resolves where the gate is parked and what it knows about
// the run. Every value has a default so a hand-run bridge still works; what the
// dispatch adds is the context that makes a listing readable.
func bridgeStoreFromEnv() (*approvals.Store, approvals.RequestContext, error) {
	dir := os.Getenv(adapter.EnvApprovalDir)
	if dir == "" {
		stateDir, err := app.DefaultStateDir()
		if err != nil {
			return nil, approvals.RequestContext{}, err
		}
		dir = filepath.Join(stateDir, "approvals")
	}
	store, err := approvals.NewStore(dir)
	if err != nil {
		return nil, approvals.RequestContext{}, err
	}

	rc := approvals.RequestContext{
		SessionID: os.Getenv(adapter.EnvTerminalSessionID),
		BeadID:    os.Getenv(adapter.EnvApprovalBeadID),
		RepoPath:  os.Getenv(adapter.EnvApprovalRepoPath),
		AgentName: os.Getenv(adapter.EnvApprovalAgentName),
		Timeout:   approvals.DefaultTimeout,
	}
	if raw := os.Getenv(adapter.EnvApprovalTimeout); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, approvals.RequestContext{}, fmt.Errorf("KERNL DISPATCH FAILURE: %s is %q, which is not a duration - Fix: use a Go duration such as 30m", adapter.EnvApprovalTimeout, raw)
		}
		rc.Timeout = parsed
	}
	return store, rc, nil
}
