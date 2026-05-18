package dispatch

import (
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type ResolvedAgent struct {
	ID   string
	Config *config.AgentConfig
}

func ResolveDispatchAgent(workflowQueueActions map[string]string, state string, agents map[string]config.AgentConfig, pools map[string]config.PoolConfig, excludeAgentIDs ...string) (*ResolvedAgent, error) {
	poolKey, ok := workflowQueueActions[state]
	if !ok {
		return nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			"pool key for state "+state,
			"add state "+state+" to workflow queueActions",
		)
	}

	pool, ok := pools[poolKey]
	if !ok {
		return nil, NewDispatchFailureError(
			poolKey, "<unknown>",
			"pool config",
			"add settings.pools."+poolKey+" with agent entries",
		)
	}

	if len(pool.Agents) == 0 {
		return nil, NewDispatchFailureError(
			poolKey, "<unknown>",
			"pool agents",
			"add agents to pool settings.pools."+poolKey,
		)
	}

	agentID, agent, err := SelectFromPoolWithID(pool, agents, excludeAgentIDs...)
	if err != nil {
		return nil, fmt.Errorf("resolveDispatchAgent for pool %q: %w", poolKey, err)
	}
	return &ResolvedAgent{ID: agentID, Config: agent}, nil
}