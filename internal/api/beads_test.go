package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

type testBackend struct {
	beads []backend.Bead
}

func (b *testBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) { return nil, nil }
func (b *testBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return b.beads, nil
}
func (b *testBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	for _, bead := range b.beads {
		if bead.ID == id {
			return &bead, nil
		}
	}
	return nil, nil
}
func (b *testBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	bead := &backend.Bead{ID: "kb-new", Title: input.Title, Priority: input.Priority}
	b.beads = append(b.beads, *bead)
	return bead, nil
}
func (b *testBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error { return nil }
func (b *testBackend) Delete(id string, repoPath string) error                                       { return nil }
func (b *testBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *testBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *testBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *testBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *testBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) AddDependency(blockerID string, blockedID string, repoPath string) error { return nil }
func (b *testBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *testBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *testBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *testBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *testBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *testBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

func testCfg() *config.Config {
	return &config.Config{
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{{Path: "/tmp/test-repo"}},
		},
	}
}

func testApp() *app.App {
	return &app.App{
		Backend: &testBackend{
			beads: []backend.Bead{{ID: "kb-1", Title: "first"}},
		},
		Config: testCfg(),
	}
}

func TestListBeadsHandlerReturnsBackendBeads(t *testing.T) {
	r := NewRouter(testApp())
	req := httptest.NewRequest("GET", "/api/beads", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var got []backend.Bead
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != "kb-1" {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestGetBeadHandlerReturnsBackendBead(t *testing.T) {
	r := NewRouter(testApp())
	req := httptest.NewRequest("GET", "/api/beads/kb-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var got backend.Bead
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.ID != "kb-1" || got.Title != "first" {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestGetBeadHandlerNotFound(t *testing.T) {
	r := NewRouter(testApp())
	req := httptest.NewRequest("GET", "/api/beads/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateBeadHandler(t *testing.T) {
	r := NewRouter(testApp())
	body := strings.NewReader(`{"title":"new bead","priority":1}`)
	req := httptest.NewRequest("POST", "/api/beads", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}
