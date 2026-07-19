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
  endpoint: http://localhost:4000
  api_key: sk-not-a-real-key

vault:
  root: /tmp/kernl-vault-fixture
  coalesceWindowMs: 250
  moveWindowMs: 900
  rescanIntervalSec: 30

inbox:
  auto_prep: true
  da_subdir: DA

server:
  port: 8080

orchestrator:
  worktreeRoot: /tmp/worktrees
  maxConcurrentBeads: 4
  runStatePath: /tmp/run-state
  stageRetryAttempts: 3

sweep:
  auto_interval_seconds: 120
  pr_stale_warn_days: 5
  failure_threshold: 3
  backoff_minutes: [5, 15]
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

// reloadSettings re-reads the config from disk, which is the only honest way to
// check what a PUT actually persisted.
func reloadSettings(t *testing.T, path string) *config.Config {
	t.Helper()

	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	return saved
}

// Each PUT is a partial update: a field the client did not send must survive
// untouched. Whole-section replacement silently blanked every omitted field.
func TestUpdateLLMLeavesOmittedFieldsAlone(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateLLMHandler(a), "/api/settings/llm", `{"model":"kimi-k2.8"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved := reloadSettings(t, path)
	if saved.LLM.Model != "kimi-k2.8" {
		t.Errorf("model = %q, want kimi-k2.8", saved.LLM.Model)
	}
	if saved.LLM.Provider != "openai" {
		t.Errorf("provider = %q, want openai — an omitted field must not be blanked", saved.LLM.Provider)
	}
	if saved.LLM.Endpoint != "http://localhost:4000" {
		t.Errorf("endpoint = %q, want it untouched — an omitted field must not be blanked", saved.LLM.Endpoint)
	}
	if saved.LLM.APIKey != "sk-not-a-real-key" {
		t.Errorf("api key = %q, want the stored credential to survive", saved.LLM.APIKey)
	}
}

func TestUpdateLLMClearsEndpointWhenExplicitlyEmptied(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateLLMHandler(a), "/api/settings/llm", `{"endpoint":""}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved := reloadSettings(t, path)
	if saved.LLM.Endpoint != "" {
		t.Errorf("endpoint = %q, want it cleared when sent as an explicit empty string", saved.LLM.Endpoint)
	}
	if saved.LLM.Provider != "openai" || saved.LLM.Model != "kimi-k2.7" {
		t.Errorf("clearing the endpoint disturbed provider/model: %q/%q", saved.LLM.Provider, saved.LLM.Model)
	}
}

// The "a model is required when a provider is set" rule has to read through to
// the stored values, or a partial update could leave the pair incoherent.
func TestUpdateLLMRejectsClearingTheModelOfAConfiguredProvider(t *testing.T) {
	a, _ := settingsApp(t)

	recorder := putSettings(t, updateLLMHandler(a), "/api/settings/llm", `{"model":""}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT = %d, want 400 — the stored provider still needs a model", recorder.Code)
	}
}

func TestUpdateVaultLeavesOmittedFieldsAlone(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateVaultHandler(a), "/api/settings/vault", `{"coalesceWindowMs":50}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved := reloadSettings(t, path)
	if saved.Vault.CoalesceWindowMs != 50 {
		t.Errorf("coalesceWindowMs = %d, want 50", saved.Vault.CoalesceWindowMs)
	}
	if saved.Vault.Root != "/tmp/kernl-vault-fixture" {
		t.Errorf("vault root = %q, want it untouched — blanking it detaches the whole substrate", saved.Vault.Root)
	}
	if saved.Vault.MoveWindowMs != 900 {
		t.Errorf("moveWindowMs = %d, want 900 — an omitted field must not be blanked", saved.Vault.MoveWindowMs)
	}
	if saved.Vault.RescanIntervalSec != 30 {
		t.Errorf("rescanIntervalSec = %d, want 30 — an omitted field must not be blanked", saved.Vault.RescanIntervalSec)
	}
}

func TestUpdateInboxLeavesOmittedFieldsAlone(t *testing.T) {
	a, path := settingsApp(t)

	// Toggling the switch alone used to 400, because the absent subdir arrived as
	// the empty string and tripped the "required" check.
	recorder := putSettings(t, updateInboxHandler(a), "/api/settings/inbox", `{"autoPrep":false}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved := reloadSettings(t, path)
	if saved.Inbox.AutoPrep {
		t.Error("autoPrep = true, want false — an explicit false must be applied")
	}
	if saved.Inbox.DASubdir != "DA" {
		t.Errorf("daSubdir = %q, want DA — an omitted field must not be blanked", saved.Inbox.DASubdir)
	}
}

func TestUpdateInboxSubdirKeepsTheAutoPrepSwitch(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateInboxHandler(a), "/api/settings/inbox", `{"daSubdir":"Briefs"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved := reloadSettings(t, path)
	if saved.Inbox.DASubdir != "Briefs" {
		t.Errorf("daSubdir = %q, want Briefs", saved.Inbox.DASubdir)
	}
	if !saved.Inbox.AutoPrep {
		t.Error("autoPrep = false, want true — an omitted bool must not be flipped off")
	}
}

func TestUpdateRuntimeLeavesOmittedFieldsAlone(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateRuntimeHandler(a), "/api/settings/runtime", `{"serverPort":9090}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved := reloadSettings(t, path)
	if saved.Server.Port != 9090 {
		t.Errorf("server port = %d, want 9090", saved.Server.Port)
	}
	if saved.Orchestrator.MaxConcurrentBeads != 4 {
		t.Errorf("maxConcurrentBeads = %d, want 4 — an omitted field must not be zeroed", saved.Orchestrator.MaxConcurrentBeads)
	}
	if saved.Orchestrator.WorktreeRoot != "/tmp/worktrees" {
		t.Errorf("worktreeRoot = %q, want it untouched", saved.Orchestrator.WorktreeRoot)
	}
	if saved.Orchestrator.RunStatePath != "/tmp/run-state" {
		t.Errorf("runStatePath = %q, want it untouched", saved.Orchestrator.RunStatePath)
	}
	if saved.Sweep.AutoIntervalSeconds != 120 {
		t.Errorf("sweep interval = %d, want 120 — an omitted field must not be zeroed", saved.Sweep.AutoIntervalSeconds)
	}
	if saved.Sweep.PRStaleWarnDays != 5 {
		t.Errorf("prStaleWarnDays = %d, want 5 — an omitted field must not be zeroed", saved.Sweep.PRStaleWarnDays)
	}
	if len(saved.Sweep.BackoffMinutes) != 2 {
		t.Errorf("backoffMinutes = %v, want the stored schedule to survive", saved.Sweep.BackoffMinutes)
	}
}

// Zero disables the periodic rescan, so a present-but-zero field is applied —
// that is the case a pointer request field exists to distinguish from absent.
func TestUpdateVaultAppliesAnExplicitZero(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateVaultHandler(a), "/api/settings/vault", `{"rescanIntervalSec":0}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	saved := reloadSettings(t, path)
	if saved.Vault.RescanIntervalSec != 0 {
		t.Errorf("rescanIntervalSec = %d, want 0 — an explicit zero must be applied", saved.Vault.RescanIntervalSec)
	}
	if saved.Vault.MoveWindowMs != 900 {
		t.Errorf("moveWindowMs = %d, want 900 — an omitted field must not be blanked", saved.Vault.MoveWindowMs)
	}
}

// The runtime ints cannot round-trip a zero, and that limit lives in the config
// layer, not here: config.Load backfills a default for every zero-valued
// orchestrator and sweep number. The API writes the zero the client asked for —
// this test pins that — but Load hands the process the default right back, so
// zero is effectively "unset" for these fields. Making one of them genuinely
// zeroable means turning it into a pointer in config.Config, which is a wider
// change than this partial-update fix.
func TestUpdateRuntimeWritesZeroButConfigLoadBackfillsIt(t *testing.T) {
	a, path := settingsApp(t)

	recorder := putSettings(t, updateRuntimeHandler(a), "/api/settings/runtime", `{"stageRetryAttempts":0}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200: %s", recorder.Code, recorder.Body)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if !strings.Contains(string(raw), "stageRetryAttempts: 0") {
		t.Errorf("the explicit zero never reached the file:\n%s", raw)
	}

	saved := reloadSettings(t, path)
	if saved.Orchestrator.StageRetryAttempts != 2 {
		t.Errorf("stageRetryAttempts = %d, want the config-layer default of 2", saved.Orchestrator.StageRetryAttempts)
	}
	if saved.Server.Port != 8080 {
		t.Errorf("server port = %d, want 8080 — an omitted field must not be zeroed", saved.Server.Port)
	}
}

// A body carrying no known field is a client bug; rewriting the config for it
// would be a no-op write of the user's file.
func TestUpdateRejectsABodyWithNothingToUpdate(t *testing.T) {
	a, _ := settingsApp(t)

	for _, section := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"llm", updateLLMHandler(a)},
		{"vault", updateVaultHandler(a)},
		{"inbox", updateInboxHandler(a)},
		{"runtime", updateRuntimeHandler(a)},
	} {
		recorder := putSettings(t, section.handler, "/api/settings/"+section.name, `{}`)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400 for an empty body", section.name, recorder.Code)
		}
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
