package dispatch

import (
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type SwapPoolResult struct {
	Pools    map[string]config.PoolConfig
	Changed  bool
	Affected int
	Updated  config.PoolConfig
}

// swapPoolEntries replaces all occurrences of fromAgentID with toAgentID in a
// single pool's entry list. When toAgentID already exists, weights are merged
// (summed) and duplicates collapsed. Returns the new entry list, or the
// original list unchanged if fromAgentID is absent or equal to toAgentID.
func swapPoolEntries(entries []config.WeightedAgent, fromAgentID, toAgentID string) []config.WeightedAgent {
	if fromAgentID == toAgentID {
		return entries
	}

	fromIndexes := make([]int, 0)
	fromWeight := 0
	for i, e := range entries {
		if e.AgentID == fromAgentID {
			fromIndexes = append(fromIndexes, i)
			fromWeight += e.Weight
		}
	}
	if len(fromIndexes) == 0 {
		return entries
	}

	removeSet := make(map[int]bool, len(fromIndexes))
	for _, idx := range fromIndexes {
		removeSet[idx] = true
	}
	firstFromIdx := fromIndexes[0]

	toIndex := -1
	for i, e := range entries {
		if e.AgentID == toAgentID {
			toIndex = i
			break
		}
	}

	if toIndex >= 0 {
		result := make([]config.WeightedAgent, 0, len(entries)-len(fromIndexes))
		for i, e := range entries {
			if removeSet[i] {
				continue
			}
			if i == toIndex {
				result = append(result, config.WeightedAgent{AgentID: toAgentID, Weight: e.Weight + fromWeight})
			} else {
				result = append(result, e)
			}
		}
		return result
	}

	result := make([]config.WeightedAgent, 0, len(entries))
	for i, e := range entries {
		if removeSet[i] && i != firstFromIdx {
			continue
		}
		if i == firstFromIdx {
			result = append(result, config.WeightedAgent{AgentID: toAgentID, Weight: fromWeight})
		} else if !removeSet[i] {
			result = append(result, e)
		}
	}
	return result
}

func SwapPoolAgent(pools map[string]config.PoolConfig, poolKey, oldAgentID, newAgentID string) SwapPoolResult {
	result := make(map[string]config.PoolConfig, len(pools))
	for k, v := range pools {
		result[k] = v
	}

	if oldAgentID == newAgentID {
		return SwapPoolResult{Pools: result, Changed: false, Affected: 0, Updated: result[poolKey]}
	}

	pool, ok := result[poolKey]
	if !ok {
		return SwapPoolResult{Pools: result, Changed: false, Affected: 0, Updated: config.PoolConfig{}}
	}

	oldCount := 0
	for _, a := range pool.Agents {
		if a.AgentID == oldAgentID {
			oldCount++
		}
	}

	if oldCount == 0 {
		return SwapPoolResult{Pools: result, Changed: false, Affected: 0, Updated: pool}
	}

	swapped := swapPoolEntries(pool.Agents, oldAgentID, newAgentID)
	pool.Agents = swapped
	result[poolKey] = pool
	return SwapPoolResult{Pools: result, Changed: true, Affected: oldCount, Updated: pool}
}

type SwapActionsResult struct {
	Actions  map[string]string
	Affected int
	Updates  map[string]string
}

func SwapActionsAgent(actions map[string]string, oldAgentID, newAgentID string) SwapActionsResult {
	result := make(map[string]string, len(actions))
	for k, v := range actions {
		result[k] = v
	}

	affected := 0
	updates := make(map[string]string)

	if oldAgentID == newAgentID {
		return SwapActionsResult{Actions: result, Affected: 0, Updates: updates}
	}

	for k, v := range result {
		if v == oldAgentID {
			result[k] = newAgentID
			affected++
			updates[k] = newAgentID
		}
	}

	return SwapActionsResult{Actions: result, Affected: affected, Updates: updates}
}

type SwapPoolsResult struct {
	Pools           map[string]config.PoolConfig
	AffectedEntries int
	AffectedSteps   int
	Updates         map[string]config.PoolConfig
}

func SwapPoolsAgent(pools map[string]config.PoolConfig, oldAgentID, newAgentID string) SwapPoolsResult {
	result := make(map[string]config.PoolConfig, len(pools))
	for k, v := range pools {
		result[k] = v
	}

	if oldAgentID == newAgentID {
		return SwapPoolsResult{Pools: result, AffectedEntries: 0, AffectedSteps: 0, Updates: map[string]config.PoolConfig{}}
	}

	totalEntries := 0
	steps := 0
	updates := make(map[string]config.PoolConfig)

	for stepKey, pool := range result {
		entryCount := 0
		for _, a := range pool.Agents {
			if a.AgentID == oldAgentID {
				entryCount++
			}
		}
		if entryCount == 0 {
			continue
		}

		swapped := swapPoolEntries(pool.Agents, oldAgentID, newAgentID)
		if len(swapped) != len(pool.Agents) || !entriesEqual(swapped, pool.Agents) {
			pool.Agents = swapped
			result[stepKey] = pool
			totalEntries += entryCount
			steps++
			updates[stepKey] = pool
		}
	}

	return SwapPoolsResult{
		Pools:           result,
		AffectedEntries: totalEntries,
		AffectedSteps:   steps,
		Updates:         updates,
	}
}

func entriesEqual(a, b []config.WeightedAgent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type CountResult struct {
	AffectedActions int
	AffectedEntries int
	AffectedSteps   int
}

func CountDispatchAgentOccurrences(actions map[string]string, pools map[string]config.PoolConfig, agentID string) CountResult {
	actionsCount := 0
	for _, v := range actions {
		if v == agentID {
			actionsCount++
		}
	}

	entriesCount := 0
	stepsCount := 0
	for _, pool := range pools {
		stepHas := false
		for _, a := range pool.Agents {
			if a.AgentID == agentID {
				entriesCount++
				stepHas = true
			}
		}
		if stepHas {
			stepsCount++
		}
	}

	return CountResult{
		AffectedActions: actionsCount,
		AffectedEntries: entriesCount,
		AffectedSteps:   stepsCount,
	}
}

func GetSwappableSourceAgentIDs(usedAgents, availableAgents []string) []string {
	availSet := make(map[string]bool, len(availableAgents))
	for _, a := range availableAgents {
		availSet[a] = true
	}

	var result []string
	for _, id := range usedAgents {
		others := 0
		for _, a := range availableAgents {
			if a != id {
				others++
			}
		}
		if others > 0 {
			result = append(result, id)
		}
	}
	return result
}

type StepAgentTracker struct {
	m map[string]string
}

func NewStepAgentTracker() *StepAgentTracker {
	return &StepAgentTracker{m: make(map[string]string)}
}

func (t *StepAgentTracker) Record(beadID, step, agentID string) {
	t.m[fmt.Sprintf("%s:%s", beadID, step)] = agentID
}

func (t *StepAgentTracker) Get(beadID, step string) (string, bool) {
	v, ok := t.m[fmt.Sprintf("%s:%s", beadID, step)]
	return v, ok
}
