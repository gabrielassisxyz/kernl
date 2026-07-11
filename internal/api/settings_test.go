package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

func TestSettingsInventoryRedactsLLMAPIKey(t *testing.T) {
	a := &app.App{Config: testSettingsConfig()}
	a.Config.LLM.APIKey = "sk-secret-value"
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/settings/inventory", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sk-secret-value") {
		t.Fatalf("inventory leaked raw API key: %s", w.Body.String())
	}

	var got settingsInventoryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	item := findSettingsItem(t, got, "llm.api_key")
	if item.Value != "Configured" {
		t.Fatalf("llm.api_key value = %q, want Configured", item.Value)
	}
	if !item.Sensitive {
		t.Fatal("llm.api_key should be marked sensitive")
	}
	if item.EditPath != "requires dedicated API" {
		t.Fatalf("llm.api_key editPath = %q", item.EditPath)
	}
}

func TestSettingsInventoryMarksEditableSources(t *testing.T) {
	got := buildSettingsInventory(testSettingsConfig())

	daItem := findSettingsItem(t, got, "da.systemPrompt")
	if !daItem.Editable || daItem.Source != "graph" || daItem.EditPath != "/api/da/identity" {
		t.Fatalf("DA item metadata = %+v", daItem)
	}

	editorItem := findSettingsItem(t, got, "editor.fontSize")
	if !editorItem.Editable || editorItem.Source != "localStorage" {
		t.Fatalf("editor item metadata = %+v", editorItem)
	}

	runtimeItem := findSettingsItem(t, got, "server.port")
	if runtimeItem.Editable || runtimeItem.Source != "yaml" || !runtimeItem.RestartRequired {
		t.Fatalf("runtime item metadata = %+v", runtimeItem)
	}
}

func testSettingsConfig() *config.Config {
	return &config.Config{
		Settings: config.Settings{
			Agents: map[string]config.AgentConfig{
				"opencode": {
					Command:      "opencode",
					Model:        "claude-sonnet-4-6",
					ApprovalMode: "auto",
				},
			},
			Pools: map[string]config.PoolConfig{
				"implementation": {
					Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}},
				},
			},
			Defaults: config.DefaultsConfig{InteractiveSessionTimeoutMinutes: 10},
		},
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{{Path: "/repo", MemoryManager: "beads"}},
		},
		Server: config.ServerConfig{Port: 8080},
		Orchestrator: config.OrchestratorConfig{
			WorktreeRoot:       "/tmp/kernl-worktrees",
			MaxConcurrentBeads: 5,
			RunStatePath:       "/tmp/kernl-runstate.db",
			StageRetryAttempts: 2,
		},
		Sweep: config.SweepConfig{
			AutoIntervalSeconds: 60,
			PRStaleWarnDays:     7,
			FailureThreshold:    3,
			BackoffMinutes:      []int{5, 15, 60},
		},
		Vault: config.VaultConfig{
			Root:              "/vault",
			CoalesceWindowMs:  300,
			MoveWindowMs:      1000,
			RescanIntervalSec: 0,
		},
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-5",
			Endpoint: "http://localhost:4000",
			APIKey:   "sk-test",
		},
		Inbox: config.InboxConfig{
			AutoPrep: true,
			DASubdir: "DA",
		},
	}
}

func findSettingsItem(t *testing.T, inventory settingsInventoryResponse, key string) settingItem {
	t.Helper()
	for _, section := range inventory.Sections {
		for _, item := range section.Items {
			if item.Key == key {
				return item
			}
		}
	}
	t.Fatalf("settings item %q not found", key)
	return settingItem{}
}
