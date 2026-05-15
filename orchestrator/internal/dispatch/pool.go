package dispatch

import (
	"fmt"
	"math/rand"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type DispatchFailureError struct {
	PoolKey string
	BeatID  string
	Missing string
	Fix     string
}

func (e *DispatchFailureError) Error() string {
	return fmt.Sprintf(
		"KERNL DISPATCH FAILURE: %s not found for pool key %q (beat %s); fix: %s",
		e.Missing, e.PoolKey, e.BeatID, e.Fix,
	)
}

func NewDispatchFailureError(poolKey, beatID, missing, fix string) *DispatchFailureError {
	return &DispatchFailureError{
		PoolKey: poolKey,
		BeatID:  beatID,
		Missing: missing,
		Fix:     fix,
	}
}

// selectByWeight performs weighted random selection among eligible entries.
// Returns the selected entry's AgentID, or "" if no eligible entries.
func selectByWeight(entries []config.WeightedAgent, rng *rand.Rand) string {
	if len(entries) == 0 {
		return ""
	}

	var totalWeight int
	for _, e := range entries {
		totalWeight += e.Weight
	}
	if totalWeight == 0 {
		return ""
	}

	r := rng.Intn(totalWeight)
	for _, e := range entries {
		r -= e.Weight
		if r < 0 {
			return e.AgentID
		}
	}

	return entries[len(entries)-1].AgentID
}

// SelectFromPool selects an agent from the pool using weighted random selection,
// excluding agents in excludeAgentIDs. It resolves the agent ID against the
// global agents registry. Dangling references (agent ID not in registry) cause
// a loud dispatch failure per spec 1.7.
func SelectFromPool(pool config.PoolConfig, agents map[string]config.AgentConfig, excludeAgentIDs ...string) (*config.AgentConfig, error) {
	filtered := filterExcluded(pool.Agents, excludeAgentIDs)
	if len(filtered) == 0 {
		return nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			"non-excluded pool agents",
			"add more agents to the pool or reduce exclusions",
		)
	}

	eligible := make([]config.WeightedAgent, 0, len(filtered))
	var danglingIDs []string
	for _, entry := range filtered {
		if _, ok := agents[entry.AgentID]; !ok {
			danglingIDs = append(danglingIDs, entry.AgentID)
			continue
		}
		if entry.Weight <= 0 {
			continue
		}
		eligible = append(eligible, entry)
	}

	if len(danglingIDs) > 0 && len(eligible) == 0 {
		return nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			fmt.Sprintf("registered agents for pool entries %v", danglingIDs),
			fmt.Sprintf("add agent configs for %v to settings.agents", danglingIDs),
		)
	}

	if len(eligible) == 0 {
		return nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			"pool agents with positive weight",
			"add agents with weight > 0 to the pool configuration",
		)
	}

	selectedID := selectByWeight(eligible, rand.New(rand.NewSource(rand.Int63())))
	cfg := agents[selectedID]
	return &cfg, nil
}

// SelectFromPoolStrict selects an agent that is NOT the excluded one.
// It returns nil if the only options are the excluded agent.
func SelectFromPoolStrict(pool config.PoolConfig, agents map[string]config.AgentConfig, excludeAgentID string) (*config.AgentConfig, error) {
	return SelectFromPool(pool, agents, excludeAgentID)
}

// SelectFromPoolWithID selects an agent and returns both agent ID and config.
func SelectFromPoolWithID(pool config.PoolConfig, agents map[string]config.AgentConfig, excludeAgentIDs ...string) (string, *config.AgentConfig, error) {
	filtered := filterExcluded(pool.Agents, excludeAgentIDs)
	if len(filtered) == 0 {
		return "", nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			"non-excluded pool agents",
			"add more agents to the pool or reduce exclusions",
		)
	}

	eligible := make([]config.WeightedAgent, 0, len(filtered))
	var danglingIDs []string
	for _, entry := range filtered {
		if _, ok := agents[entry.AgentID]; !ok {
			danglingIDs = append(danglingIDs, entry.AgentID)
			continue
		}
		if entry.Weight <= 0 {
			continue
		}
		eligible = append(eligible, entry)
	}

	if len(danglingIDs) > 0 && len(eligible) == 0 {
		return "", nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			fmt.Sprintf("registered agents for pool entries %v", danglingIDs),
			fmt.Sprintf("add agent configs for %v to settings.agents", danglingIDs),
		)
	}

	if len(eligible) == 0 {
		return "", nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			"pool agents with positive weight",
			"add agents with weight > 0 to the pool configuration",
		)
	}

	selectedID := selectByWeight(eligible, rand.New(rand.NewSource(rand.Int63())))
	cfg, ok := agents[selectedID]
	if !ok {
		return "", nil, NewDispatchFailureError(
			"<unknown>", "<unknown>",
			"agent config for "+selectedID,
			"add agent config for "+selectedID+" to settings.agents",
		)
	}
	return selectedID, &cfg, nil
}

func filterExcluded(entries []config.WeightedAgent, excludeIDs []string) []config.WeightedAgent {
	var result []config.WeightedAgent
	for _, e := range entries {
		excluded := false
		for _, id := range excludeIDs {
			if e.AgentID == id {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, e)
		}
	}
	return result
}