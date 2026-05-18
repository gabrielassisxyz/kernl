package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/epic"
)

func TestEpicEventsHandlerStreamsExecutorEvents(t *testing.T) {
	hub := epic.NewEpicEventHub()
	a := &app.App{
		Backend:    &testBackend{},
		Config:     testCfg(),
		EpicEvents: hub,
	}
	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/api/epics/e/events", nil)
	w := httptest.NewRecorder()
	go func() {
		hub.Publish(epic.EpicEvent{Type: epic.SessionStarted, EpicID: "e", BeadID: "c1"})
		hub.Close("e")
	}()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "SessionStarted") || !strings.Contains(w.Body.String(), "c1") {
		t.Errorf("SSE body missing event: %q", w.Body.String())
	}
}
