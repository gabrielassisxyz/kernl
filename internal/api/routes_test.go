package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/epic"
)

func TestCORSReflectsAllowedLocalOrigins(t *testing.T) {
	r := NewRouter(testApp())
	for _, origin := range []string{
		"http://localhost:3000",
		"http://127.0.0.1:8080",
		"http://localhost",
		"https://localhost:5173",
		"http://[::1]:3000",
	} {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %s: expected it reflected, got %q", origin, got)
		}
	}
}

func TestCORSRejectsForeignOrigins(t *testing.T) {
	r := NewRouter(testApp())
	for _, origin := range []string{
		"https://evil.example",
		"http://evil.example",
		"https://localhost.evil.example",
		"http://127.0.0.1.evil.example",
		"null",
	} {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %s: expected no allow-origin header, got %q", origin, got)
		}
	}
}

func TestCORSPreflightForAllowedOrigin(t *testing.T) {
	r := NewRouter(testApp())
	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 preflight, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("expected reflected origin on preflight, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("expected POST in allow-methods, got %q", got)
	}
}

func TestCORSPreflightForForeignOriginCarriesNoGrant(t *testing.T) {
	r := NewRouter(testApp())
	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no allow-origin on foreign preflight, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Errorf("expected no allow-methods on foreign preflight, got %q", got)
	}
}

// Vary must be set even when the origin is rejected: a shared cache that stored
// the header-less response must not replay it for an allowed origin.
func TestCORSAlwaysVariesOnOrigin(t *testing.T) {
	r := NewRouter(testApp())
	for _, origin := range []string{"http://localhost:3000", "https://evil.example", ""} {
		req := httptest.NewRequest("GET", "/api/health", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if got := w.Header().Values("Vary"); !containsFold(got, "Origin") {
			t.Errorf("origin %q: expected Vary: Origin, got %v", origin, got)
		}
	}
}

// The SSE routes carry live agent output and terminal streams — the most
// valuable thing on this unauthenticated API. A handler that sets its own
// wildcard would override the middleware's policy for exactly those routes.
func TestSSERoutesDoNotGrantForeignOrigins(t *testing.T) {
	a := testApp()
	a.EpicEvents = epic.NewEpicEventHub()
	r := NewRouter(a)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the stream ends immediately; only the response headers matter here
	req := httptest.NewRequest("GET", "/api/epics/e1/events", nil).WithContext(ctx)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Guard against a vacuous pass: a 500 from an unconfigured hub would never
	// reach the streaming handler that sets the header under test.
	if w.Code != http.StatusOK {
		t.Fatalf("expected the SSE handler to run, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no allow-origin for a foreign origin, got %q", got)
	}
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), want) {
				return true
			}
		}
	}
	return false
}
