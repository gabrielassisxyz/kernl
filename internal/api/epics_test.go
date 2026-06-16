package api

import (
	"net/http"
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

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Pre-publish event so it is buffered and immediately served upon subscription
	hub.Publish(epic.EpicEvent{Type: epic.SessionStarted, EpicID: "e", BeadID: "c1"})

	resp, err := http.Get(ts.URL + "/api/epics/e/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil {
		t.Fatalf("failed to read stream: %v", err)
	}
	body := string(buf[:n])

	if !strings.Contains(body, "SessionStarted") || !strings.Contains(body, "c1") {
		t.Errorf("SSE body missing event: %q", body)
	}
}
