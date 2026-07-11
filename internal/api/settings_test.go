package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/vault"
)

const settingsFixture = `settings:
  agents:
    opencode:
      command: opencode

registry:
  repos:
    - path: /tmp/repo

# Keep this comment: the writer must not eat it.
llm:
  provider: openai
  model: kimi-k2.7
  api_key: sk-not-a-real-key
`

// settingsApp builds an App around a real config file on disk, which is the only
// way to exercise the write path honestly.
func settingsApp(t *testing.T) (*app.App, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kernl.yaml")
	if err := os.WriteFile(path, []byte(settingsFixture), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("loading fixture: %v", err)
	}
	// serve normalizes the vault windows into the running config right after boot,
	// so the App under test has to start from the same place the real one does.
	vault.ApplyDefaults(&cfg.Vault)
	return &app.App{Config: cfg, ConfigPath: path}, path
}

func getSettings(t *testing.T, a *app.App) settingsResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	settingsHandler(a)(recorder, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/settings = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	var response settingsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return response
}

func putSettings(t *testing.T, handler http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))
	return recorder
}

func TestSettingsNeverSerializesTheAPIKey(t *testing.T) {
	a, _ := settingsApp(t)

	recorder := httptest.NewRecorder()
	settingsHandler(a)(recorder, httptest.NewRequest(http.MethodGet, "/api/settings", nil))

	if strings.Contains(recorder.Body.String(), "sk-not-a-real-key") {
		t.Fatal("the raw API key leaked into the settings response")
	}

	if !getSettings(t, a).LLM.APIKeySet {
		t.Error("apiKeySet should report that a credential exists")
	}
}

func TestUpdateLLMPersistsAndKeepsUnretypedKey(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateLLMHandler(a), "/api/settings/llm",
		`{"provider":"openai","model":"kimi-k2.8","endpoint":"http://localhost:4000"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if saved.LLM.Model != "kimi-k2.8" {
		t.Errorf("model = %q, want kimi-k2.8", saved.LLM.Model)
	}
	if saved.LLM.Endpoint != "http://localhost:4000" {
		t.Errorf("endpoint = %q, want http://localhost:4000", saved.LLM.Endpoint)
	}
	// The UI never echoes the key back, so an omitted apiKey must not wipe it.
	if saved.LLM.APIKey != "sk-not-a-real-key" {
		t.Errorf("api key = %q, want the stored key to survive an update that omitted it", saved.LLM.APIKey)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if !strings.Contains(string(raw), "# Keep this comment: the writer must not eat it.") {
		t.Error("the write destroyed a user comment")
	}
}

func TestUpdateLLMClearsKeyWhenExplicitlyEmptied(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateLLMHandler(a), "/api/settings/llm",
		`{"provider":"openai","model":"kimi-k2.7","apiKey":""}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if saved.LLM.APIKey != "" {
		t.Errorf("api key = %q, want it cleared when the client sent an explicit empty string", saved.LLM.APIKey)
	}
}

func TestUpdateLLMRejectsUnknownProvider(t *testing.T) {
	a, path := settingsApp(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	recorder := putSettings(t, updateLLMHandler(a), "/api/settings/llm",
		`{"provider":"gpt-cloud","model":"whatever"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT = %d, want 400", recorder.Code)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if string(after) != string(before) {
		t.Error("a rejected update still touched the config file")
	}
}

func TestUpdateRuntimeRejectsOutOfRangeValues(t *testing.T) {
	a, _ := settingsApp(t)

	body := `{"serverPort":70000,"worktreeRoot":"/tmp/wt","maxConcurrentBeads":5,` +
		`"runStatePath":"/tmp/run.db","stageRetryAttempts":2,"sweepIntervalSec":60,` +
		`"prStaleWarnDays":7,"sweepFailureLimit":3,"sweepBackoffMinutes":[5,15]}`

	recorder := putSettings(t, updateRuntimeHandler(a), "/api/settings/runtime", body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT = %d, want 400 for an out-of-range port", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "port") {
		t.Errorf("error should name the offending field, got: %s", recorder.Body)
	}
}

func TestUpdateInboxRejectsEscapingSubdir(t *testing.T) {
	a, _ := settingsApp(t)

	recorder := putSettings(t, updateInboxHandler(a), "/api/settings/inbox",
		`{"autoPrep":true,"daSubdir":"../../etc"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT = %d, want 400 for a subdir that climbs out of the vault", recorder.Code)
	}
}

// serve normalizes the vault windows into the running config after boot. If the
// settings API re-reads the file without the same normalization, an untouched
// config reports its own defaults as unsaved changes.
func TestBootDefaultsAreNotReportedAsPendingChanges(t *testing.T) {
	a, _ := settingsApp(t)

	if pending := getSettings(t, a).RestartPending; len(pending) != 0 {
		t.Errorf("restartPending = %v, want none — nothing was written", pending)
	}
}

func TestSavedChangeIsReportedAsRestartPending(t *testing.T) {
	a, _ := settingsApp(t)

	if pending := getSettings(t, a).RestartPending; len(pending) != 0 {
		t.Fatalf("a freshly loaded config should have nothing pending, got %v", pending)
	}

	recorder := putSettings(t, updateLLMHandler(a), "/api/settings/llm",
		`{"provider":"openai","model":"kimi-k2.8"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	pending := getSettings(t, a).RestartPending
	if len(pending) != 1 || pending[0] != "llm.model" {
		t.Errorf("restartPending = %v, want [llm.model] — the running process still holds the old value", pending)
	}
}

func TestWritesAreRefusedWithoutAConfigFile(t *testing.T) {
	a, _ := settingsApp(t)
	a.ConfigPath = ""

	recorder := putSettings(t, updateInboxHandler(a), "/api/settings/inbox",
		`{"autoPrep":true,"daSubdir":"DA"}`)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("PUT = %d, want 409 when there is no file to write to", recorder.Code)
	}
}
