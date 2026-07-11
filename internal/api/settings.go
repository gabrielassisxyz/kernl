package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

type settingsInventoryResponse struct {
	Summary  []settingsSummaryItem `json:"summary"`
	Sections []settingsSection     `json:"sections"`
}

type settingsSummaryItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Value       string `json:"value"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

type settingsSection struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Items       []settingItem `json:"items"`
}

type settingItem struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Description     string `json:"description"`
	Value           string `json:"value"`
	Source          string `json:"source"`
	Status          string `json:"status"`
	EditPath        string `json:"editPath"`
	Editable        bool   `json:"editable"`
	Sensitive       bool   `json:"sensitive"`
	RestartRequired bool   `json:"restartRequired"`
}

func RegisterSettingsRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/settings/inventory", settingsInventoryHandler(a))
}

func settingsInventoryHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if a.Config == nil {
			writeError(w, http.StatusServiceUnavailable, "settings inventory requires a loaded config")
			return
		}

		_ = json.NewEncoder(w).Encode(buildSettingsInventory(a.Config))
	}
}

func buildSettingsInventory(cfg *config.Config) settingsInventoryResponse {
	sections := []settingsSection{
		buildDASettingsSection(),
		buildPromptSettingsSection(),
		buildEditorSettingsSection(),
		buildRuntimeSettingsSection(cfg),
		buildAgentSettingsSection(cfg),
		buildLLMSettingsSection(cfg),
		buildVaultSettingsSection(cfg),
		buildInboxSettingsSection(cfg),
	}

	return settingsInventoryResponse{
		Summary:  buildSettingsSummary(cfg),
		Sections: sections,
	}
}

func buildPromptSettingsSection() settingsSection {
	return settingsSection{
		ID:          "prompts",
		Title:       "Prompts",
		Description: "Prompt surfaces that influence assistant and AI-assisted workflows.",
		Items: []settingItem{
			graphEditableItem("prompts.da.systemPrompt", "DA system prompt", "Primary assistant instruction. Edit this from the DA tab.", "/api/da/identity"),
			{
				Key:         "prompts.notes.applyInstruction",
				Label:       "Note edit instruction",
				Description: "Prompt used when applying an AI edit instruction to a note body.",
				Value:       "Hardcoded",
				Source:      "hardcoded",
				Status:      "readOnly",
				EditPath:    "requires dedicated API",
			},
			{
				Key:         "prompts.inbox.classifier",
				Label:       "Inbox classifier",
				Description: "Prompt family used to classify captures and suggest next actions.",
				Value:       "Hardcoded",
				Source:      "hardcoded",
				Status:      "readOnly",
				EditPath:    "requires dedicated API",
			},
			{
				Key:         "prompts.inbox.prep",
				Label:       "Inbox prep",
				Description: "Prompt family used to prepare DA-authored brief notes from captures.",
				Value:       "Hardcoded",
				Source:      "hardcoded",
				Status:      "readOnly",
				EditPath:    "requires dedicated API",
			},
		},
	}
}

func buildSettingsSummary(cfg *config.Config) []settingsSummaryItem {
	vaultStatus := "missing"
	vaultValue := "Disabled"
	if cfg.Vault.Root != "" {
		vaultStatus = "configured"
		vaultValue = "Configured"
	}

	llmStatus := "missing"
	llmValue := "Disabled"
	if cfg.LLM.IsSet() {
		llmStatus = "configured"
		llmValue = cfg.LLM.Provider
		if cfg.LLM.Model != "" {
			llmValue = cfg.LLM.Provider + " / " + cfg.LLM.Model
		}
	}

	return []settingsSummaryItem{
		{
			ID:          "da",
			Label:       "DA identity",
			Value:       "Editable",
			Status:      "configured",
			Description: "Stored in the graph and editable from Settings.",
		},
		{
			ID:          "editor",
			Label:       "Editor preferences",
			Value:       "Local",
			Status:      "configured",
			Description: "Stored in browser localStorage for this device.",
		},
		{
			ID:          "vault",
			Label:       "Vault",
			Value:       vaultValue,
			Status:      vaultStatus,
			Description: "Loaded from kernl.yaml at server start.",
		},
		{
			ID:          "llm",
			Label:       "LLM",
			Value:       llmValue,
			Status:      llmStatus,
			Description: "Provider settings are read from kernl.yaml.",
		},
		{
			ID:          "agents",
			Label:       "Agents",
			Value:       fmt.Sprintf("%d configured", len(cfg.Settings.Agents)),
			Status:      configuredStatus(len(cfg.Settings.Agents) > 0),
			Description: "Dispatch agents and pools are loaded from kernl.yaml.",
		},
	}
}

func buildDASettingsSection() settingsSection {
	return settingsSection{
		ID:          "da",
		Title:       "DA identity",
		Description: "Assistant name and system prompt stored in the graph.",
		Items: []settingItem{
			graphEditableItem("da.displayName", "Display name", "Human-facing assistant name.", "/api/da/identity"),
			graphEditableItem("da.systemPrompt", "System prompt", "Base instruction used by the local assistant.", "/api/da/identity"),
		},
	}
}

func buildEditorSettingsSection() settingsSection {
	return settingsSection{
		ID:          "editor",
		Title:       "Editor preferences",
		Description: "Browser-local note editor preferences.",
		Items: []settingItem{
			localEditableItem("editor.viewMode", "View mode", "Source, live preview, or reading mode."),
			localEditableItem("editor.lineNumbers", "Line numbers", "Show line numbers in source/live editor modes."),
			localEditableItem("editor.typewriter", "Typewriter mode", "Keep the active line near the visual center."),
			localEditableItem("editor.showId", "Show note ID", "Expose the graph node ID in note properties."),
			localEditableItem("editor.font", "Editor font", "Sans, serif, or mono reading/editing font."),
			localEditableItem("editor.fontSize", "Font size", "Local editor text size."),
			localEditableItem("editor.headingScale", "Heading scale", "Local markdown heading scale."),
		},
	}
}

func buildRuntimeSettingsSection(cfg *config.Config) settingsSection {
	return settingsSection{
		ID:          "runtime",
		Title:       "Runtime",
		Description: "Server and orchestrator values loaded from kernl.yaml.",
		Items: []settingItem{
			yamlReadOnlyItem("server.port", "Server port", "HTTP API and UI port.", fmt.Sprintf("%d", cfg.Server.Port), true),
			yamlReadOnlyItem("orchestrator.worktreeRoot", "Worktree root", "Base directory for per-bead git worktrees.", cfg.Orchestrator.WorktreeRoot, true),
			yamlReadOnlyItem("orchestrator.maxConcurrentBeads", "Max concurrent beads", "Maximum parallel beads in one epic wave.", fmt.Sprintf("%d", cfg.Orchestrator.MaxConcurrentBeads), true),
			yamlReadOnlyItem("orchestrator.runStatePath", "Run-state path", "SQLite run-state database path.", cfg.Orchestrator.RunStatePath, true),
			yamlReadOnlyItem("orchestrator.stageRetryAttempts", "Stage retry attempts", "Retry budget for stage execution.", fmt.Sprintf("%d", cfg.Orchestrator.StageRetryAttempts), true),
			yamlReadOnlyItem("sweep.auto_interval_seconds", "Sweep interval", "Automatic sweep cadence in seconds.", fmt.Sprintf("%d", cfg.Sweep.AutoIntervalSeconds), true),
			yamlReadOnlyItem("sweep.pr_stale_warn_days", "PR stale warning", "Days before sweep flags a stale PR.", fmt.Sprintf("%d", cfg.Sweep.PRStaleWarnDays), true),
			yamlReadOnlyItem("sweep.failure_threshold", "Failure threshold", "Failure count before backoff behavior escalates.", fmt.Sprintf("%d", cfg.Sweep.FailureThreshold), true),
			yamlReadOnlyItem("sweep.backoff_minutes", "Backoff minutes", "Retry backoff schedule.", intSliceValue(cfg.Sweep.BackoffMinutes), true),
		},
	}
}

func buildAgentSettingsSection(cfg *config.Config) settingsSection {
	agentIDs := sortedKeys(cfg.Settings.Agents)
	poolIDs := sortedKeys(cfg.Settings.Pools)

	return settingsSection{
		ID:          "agents",
		Title:       "Agents",
		Description: "Dispatch agents, action bindings, and weighted pools from kernl.yaml.",
		Items: []settingItem{
			yamlReadOnlyItem("settings.agents", "Agents", "Configured CLI agents.", strings.Join(agentIDs, ", "), true),
			yamlReadOnlyItem("settings.pools", "Pools", "Weighted dispatch pools.", strings.Join(poolIDs, ", "), true),
			yamlReadOnlyItem("settings.actions.take", "Take action agent", "Agent used for take-loop sessions when configured.", cfg.Settings.Actions.Take, true),
			yamlReadOnlyItem("settings.actions.scene", "Scene action agent", "Agent used for scene sessions when configured.", cfg.Settings.Actions.Scene, true),
			yamlReadOnlyItem("settings.actions.scopeRefinement", "Scope refinement agent", "Agent used to refine bead scope.", cfg.Settings.Actions.ScopeRefinement, true),
			yamlReadOnlyItem("settings.actions.staleGrooming", "Stale grooming agent", "Agent used for stale-work grooming.", cfg.Settings.Actions.StaleGrooming, true),
			yamlReadOnlyItem("settings.defaults.interactiveSessionTimeoutMinutes", "Interactive session timeout", "Default timeout for human-waiting sessions.", fmt.Sprintf("%d minutes", cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes), true),
		},
	}
}

func buildLLMSettingsSection(cfg *config.Config) settingsSection {
	apiKeyValue := "Not set"
	if cfg.LLM.APIKey != "" {
		apiKeyValue = "Configured"
	}

	return settingsSection{
		ID:          "llm",
		Title:       "LLM",
		Description: "Provider settings used by chat, ingest, and note AI features.",
		Items: []settingItem{
			yamlReadOnlyItem("llm.provider", "Provider", "LLM provider name.", cfg.LLM.Provider, true),
			yamlReadOnlyItem("llm.model", "Model", "Model used by the configured provider.", cfg.LLM.Model, true),
			yamlReadOnlyItem("llm.endpoint", "Endpoint", "Optional custom provider endpoint.", cfg.LLM.Endpoint, true),
			{
				Key:             "llm.api_key",
				Label:           "API key",
				Description:     "Provider credential. The raw value is never returned by the API.",
				Value:           apiKeyValue,
				Source:          "yaml",
				Status:          "readOnly",
				EditPath:        "requires dedicated API",
				Sensitive:       true,
				RestartRequired: true,
			},
		},
	}
}

func buildVaultSettingsSection(cfg *config.Config) settingsSection {
	return settingsSection{
		ID:          "vault",
		Title:       "Vault",
		Description: "Notes vault watcher and graph integration.",
		Items: []settingItem{
			yamlReadOnlyItem("vault.root", "Vault root", "Absolute path to the notes vault.", cfg.Vault.Root, true),
			yamlReadOnlyItem("vault.coalesceWindowMs", "Coalesce window", "Filesystem quiet period before emitting a change.", fmt.Sprintf("%d ms", cfg.Vault.CoalesceWindowMs), true),
			yamlReadOnlyItem("vault.moveWindowMs", "Move window", "Move/delete correlation window.", fmt.Sprintf("%d ms", cfg.Vault.MoveWindowMs), true),
			yamlReadOnlyItem("vault.rescanIntervalSec", "Rescan interval", "Periodic full rescan interval. Zero disables periodic rescans.", fmt.Sprintf("%d seconds", cfg.Vault.RescanIntervalSec), true),
		},
	}
}

func buildInboxSettingsSection(cfg *config.Config) settingsSection {
	return settingsSection{
		ID:          "inbox",
		Title:       "Inbox",
		Description: "DA-assisted inbox preprocessing.",
		Items: []settingItem{
			yamlReadOnlyItem("inbox.auto_prep", "Auto prep", "Let the classifier proactively generate primers for question-like captures.", fmt.Sprintf("%t", cfg.Inbox.AutoPrep), true),
			yamlReadOnlyItem("inbox.da_subdir", "DA subdirectory", "Vault folder for DA-authored prep notes.", cfg.Inbox.DASubdir, true),
		},
	}
}

func graphEditableItem(key, label, description, editPath string) settingItem {
	return settingItem{
		Key:         key,
		Label:       label,
		Description: description,
		Value:       "Configured",
		Source:      "graph",
		Status:      "editable",
		EditPath:    editPath,
		Editable:    true,
	}
}

func localEditableItem(key, label, description string) settingItem {
	return settingItem{
		Key:         key,
		Label:       label,
		Description: description,
		Value:       "Local preference",
		Source:      "localStorage",
		Status:      "editable",
		EditPath:    "browser localStorage",
		Editable:    true,
	}
}

func yamlReadOnlyItem(key, label, description, value string, restartRequired bool) settingItem {
	if value == "" {
		value = "Not set"
	}
	return settingItem{
		Key:             key,
		Label:           label,
		Description:     description,
		Value:           value,
		Source:          "yaml",
		Status:          "readOnly",
		EditPath:        "requires dedicated API",
		RestartRequired: restartRequired,
	}
}

func configuredStatus(configured bool) string {
	if configured {
		return "configured"
	}
	return "missing"
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func intSliceValue(values []int) string {
	if len(values) == 0 {
		return "Not set"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return strings.Join(parts, ", ")
}
