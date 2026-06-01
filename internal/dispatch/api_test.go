package dispatch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

type dummyBackend struct {
	backend.BackendPort
}

func (d *dummyBackend) Get(id, repo string) (*backend.Bead, error) {
	return &backend.Bead{ID: id}, nil
}

func TestHandleEpicRunAPI(t *testing.T) {
	cfg := &config.Config{
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{{Path: "/tmp/fake"}},
		},
	}
	be := &dummyBackend{}
	handler := HandleEpicRunAPI(be, cfg)

	req := httptest.NewRequest("POST", "/api/epics/e1/run", nil)
	req.SetPathValue("id", "e1")

	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestInferWorkflowNoLLM(t *testing.T) {
	_, err := InferWorkflow(context.Background(), config.LLMConfig{}, &backend.Bead{})
	if err == nil {
		t.Errorf("expected error when LLM config is not set")
	}
}
