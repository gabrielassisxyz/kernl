package app

import (
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
)

// ResolveAgentForPool picks a concrete agent from the named pool using
// dispatch.SelectFromPoolWithID (weighted random over settings.pools[pool]),
// and returns a RunBeadInput populated with the spawn command + args + env.
//
// BeadID and RepoPath are left empty — the caller fills them in per-bead.
//
// If the selected agent has a non-empty Model field, a trailing
// "--model <model>" arg is appended so opencode (and most other agent CLIs)
// receive the correct upstream model identifier (e.g. litellm/deepseek-v4-pro-high).
//
// Returns a wrapped DispatchFailureError when the pool is missing or has no
// eligible agents.
func ResolveAgentForPool(cfg *config.Config, pool string) (RunBeadInput, error) {
	if cfg == nil {
		return RunBeadInput{}, fmt.Errorf("KERNL DISPATCH FAILURE: ResolveAgentForPool called with nil config")
	}
	poolCfg, ok := cfg.Settings.Pools[pool]
	if !ok {
		return RunBeadInput{}, fmt.Errorf("KERNL DISPATCH FAILURE: pool %q not declared in settings.pools — Fix: add settings.pools.%s with at least one agent entry", pool, pool)
	}

	agentID, agentCfg, err := dispatch.SelectFromPoolWithID(poolCfg, cfg.Settings.Agents)
	if err != nil {
		return RunBeadInput{}, fmt.Errorf("KERNL DISPATCH FAILURE: selecting agent from pool %q: %w", pool, err)
	}

	args := copySlice(agentCfg.Args)
	if agentCfg.Model != "" {
		// Append --model so the CLI binary knows which upstream provider/model to
		// use. This is supported by opencode and most litellm-proxy consumers.
		args = append(args, "--model", agentCfg.Model)
	}

	return RunBeadInput{
		Command:   agentCfg.Command,
		Args:      args,
		Env:       agentCfg.Env,
		AgentName: agentID,
	}, nil
}

// ResolveAgentForBead performs workflow-aware dispatch: it fetches the bead's
// current state, resolves the appropriate workflow, derives the pool key from
// the state-machine dispatch tables, and selects the agent.
//
// Use this instead of ResolveAgentForPool when you want the correct pool per
// state (e.g. plan_review -> plan_review pool, implementation -> implementation
// pool).
func ResolveAgentForBead(cfg *config.Config, be backend.BackendPort, beadID, repoPath string) (RunBeadInput, error) {
	if cfg == nil {
		return RunBeadInput{}, fmt.Errorf("KERNL DISPATCH FAILURE: ResolveAgentForBead called with nil config")
	}

	bead, err := be.Get(beadID, repoPath)
	if err != nil || bead == nil {
		return RunBeadInput{}, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in repo %s: %w", beadID, repoPath, err)
	}

	wf := backend.ResolveWorkflow(bead)
	poolKey := dispatch.DerivePoolKey(&wf, bead.State)
	if poolKey == "" {
		poolKey = "implementation"
	}

	return ResolveAgentForPool(cfg, poolKey)
}

func copySlice(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}
