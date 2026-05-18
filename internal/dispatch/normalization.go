package dispatch

import (
	"regexp"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type SettingsNormalizationResult struct {
	Settings     config.Settings
	ChangedPaths []string
}

var claudeModelDotRe = regexp.MustCompile(`^(?i)claude-(opus|sonnet|haiku)-(\d+(?:[-.]\d+)*)(.*)$`)

func CleanString(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	return t
}

func CanonicalizeRuntimeModel(command, rawModel string) string {
	cleaned := CleanString(rawModel)
	if cleaned == "" {
		return ""
	}

	providerID := DetectAgentProviderId(command)
	if providerID == ProviderClaude {
		lower := strings.ToLower(cleaned)
		m := claudeModelDotRe.FindStringSubmatch(lower)
		if m != nil {
			version := strings.ReplaceAll(m[2], ".", "-")
			return "claude-" + m[1] + "-" + version + m[3]
		}
	}

	return cleaned
}

func NormalizeRegisteredAgentConfig(agent config.AgentConfig) config.AgentConfig {
	command := CleanString(agent.Command)
	if command == "" {
		command = agent.Command
	}

	runtimeModel := CanonicalizeRuntimeModel(command, agent.Model)

	identity := AgentIdentityLike{
		Command:    command,
		AgentType:  CleanString(agent.Type),
		Vendor:     CleanString(agent.Vendor),
		Provider:   CleanString(agent.Provider),
		AgentName:  CleanString(agent.AgentName),
		LeaseModel: CleanString(agent.LeaseModel),
		Model:      runtimeModel,
		Flavor:     CleanString(agent.Flavor),
		Version:    CleanString(agent.Version),
		Kind:       CleanString(agent.Type),
	}

	canonical := ToCanonicalLeaseIdentity(identity)
	normalized := NormalizeAgentIdentity(identity)

	label := FormatAgentDisplayLabel(AgentIdentityLike{
		Command:  command,
		Provider: normalized.Provider,
		Model:    runtimeModel,
		Flavor:   normalized.Flavor,
		Version:  normalized.Version,
	})

	result := config.AgentConfig{
		Command: command,
	}

	if canonical.AgentType != "" {
		result.Type = canonical.AgentType
	}
	if canonical.Vendor != "" && canonical.Vendor != "unknown" {
		result.Vendor = canonical.Vendor
	}
	if canonical.Provider != "" {
		result.Provider = canonical.Provider
	}
	if canonical.AgentName != "" {
		result.AgentName = canonical.AgentName
	}
	if canonical.LeaseModel != "" {
		result.LeaseModel = canonical.LeaseModel
	}
	if runtimeModel != "" {
		result.Model = runtimeModel
	}
	if normalized.Flavor != "" {
		result.Flavor = normalized.Flavor
	}
	if normalized.Version != "" {
		result.Version = normalized.Version
	}
	if agent.ApprovalMode != "" {
		result.ApprovalMode = agent.ApprovalMode
	}
	if label != "" {
		result.Label = label
	}

	return result
}

var canonicalFields = []string{
	"command", "type", "vendor", "provider", "agent_name",
	"lease_model", "model", "version", "flavor", "label",
}

func NormalizeSettingsAgents(current config.Settings) SettingsNormalizationResult {
	normalizedAgents := make(map[string]config.AgentConfig, len(current.Agents))
	var changedPaths []string

	for agentID, rawAgent := range current.Agents {
		normalizedAgent := NormalizeRegisteredAgentConfig(rawAgent)
		normalizedAgents[agentID] = normalizedAgent

		for _, key := range canonicalFields {
			before := fieldStringValue(rawAgent, key)
			after := fieldStringValue(normalizedAgent, key)
			if before != after {
				changedPaths = append(changedPaths, "agents."+agentID+"."+key)
			}
		}

		if rawAgent.Args != nil || normalizedAgent.Args != nil {
			changedPaths = append(changedPaths, "agents."+agentID+".args")
		}
		if rawAgent.Env != nil || normalizedAgent.Env != nil {
			changedPaths = append(changedPaths, "agents."+agentID+".env")
		}
		if rawAgent.ApprovalMode != normalizedAgent.ApprovalMode {
			changedPaths = append(changedPaths, "agents."+agentID+".approvalMode")
		}
	}

	normalized := config.Settings{
		Agents:   normalizedAgents,
		Actions:  current.Actions,
		Pools:    current.Pools,
		Defaults: current.Defaults,
	}

	registeredIDs := make(map[string]bool, len(normalizedAgents))
	for id := range normalizedAgents {
		registeredIDs[id] = true
	}

	pruneOrphanActionRefs(&normalized, registeredIDs, &changedPaths)
	pruneOrphanPoolRefs(&normalized, registeredIDs, &changedPaths)

	dedup := make(map[string]bool, len(changedPaths))
	uniquePaths := make([]string, 0, len(changedPaths))
	for _, p := range changedPaths {
		if !dedup[p] {
			dedup[p] = true
			uniquePaths = append(uniquePaths, p)
		}
	}

	return SettingsNormalizationResult{
		Settings:     normalized,
		ChangedPaths: uniquePaths,
	}
}

var actionKeys = []string{"take", "scene", "scopeRefinement", "staleGrooming"}

func pruneOrphanActionRefs(root *config.Settings, registeredIDs map[string]bool, changedPaths *[]string) {
	if len(registeredIDs) == 0 {
		return
	}
	actionRefs := map[string]*string{
		"take":             &root.Actions.Take,
		"scene":            &root.Actions.Scene,
		"scopeRefinement":  &root.Actions.ScopeRefinement,
		"staleGrooming":    &root.Actions.StaleGrooming,
	}
	for _, key := range actionKeys {
		ptr, ok := actionRefs[key]
		if !ok {
			continue
		}
		value := *ptr
		if value == "" {
			continue
		}
		if !registeredIDs[value] {
			*ptr = ""
			*changedPaths = append(*changedPaths, "actions."+key)
		}
	}
}

func pruneOrphanPoolRefs(root *config.Settings, registeredIDs map[string]bool, changedPaths *[]string) {
	if len(registeredIDs) == 0 {
		return
	}
	for step, pool := range root.Pools {
		filtered := make([]config.WeightedAgent, 0, len(pool.Agents))
		changed := false
		for _, entry := range pool.Agents {
			if !registeredIDs[entry.AgentID] {
				changed = true
				continue
			}
			filtered = append(filtered, entry)
		}
		if changed {
			pool.Agents = filtered
			root.Pools[step] = pool
			*changedPaths = append(*changedPaths, "pools."+step)
		}
	}
}

func fieldStringValue(a config.AgentConfig, field string) string {
	switch field {
	case "command":
		return CleanString(a.Command)
	case "type":
		return CleanString(a.Type)
	case "vendor":
		return CleanString(a.Vendor)
	case "provider":
		return CleanString(a.Provider)
	case "agent_name":
		return CleanString(a.AgentName)
	case "lease_model":
		return CleanString(a.LeaseModel)
	case "model":
		return CleanString(a.Model)
	case "version":
		return CleanString(a.Version)
	case "flavor":
		return CleanString(a.Flavor)
	case "label":
		return CleanString(a.Label)
	default:
		return ""
	}
}