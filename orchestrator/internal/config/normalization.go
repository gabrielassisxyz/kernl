package config

// Agent config normalization and orphan pruning logic lives in internal/dispatch
// (dispatch/normalization.go) to avoid import cycles, since it depends on
// dispatch.ToCanonicalLeaseIdentity and dispatch.FormatAgentDisplayLabel.
// Use dispatch.NormalizeSettingsAgents and dispatch.NormalizeRegisteredAgentConfig.