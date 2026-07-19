package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The route must never assert an update state it did not check. It reports
// "unknown" so a client can branch on it instead of showing a false all-clear.
func TestAppUpdateDoesNotClaimUpToDate(t *testing.T) {
	r := NewRouter(testApp())
	req := httptest.NewRequest("GET", "/api/app-update", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Status  string `json:"status"`
		Checked bool   `json:"checked"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, w.Body.String())
	}

	if body.Status == "up_to_date" {
		t.Errorf("route claims up_to_date without checking any release feed")
	}
	if body.Status != "unknown" {
		t.Errorf("expected status %q, got %q", "unknown", body.Status)
	}
	if body.Checked {
		t.Errorf("expected checked=false — nothing contacts a release feed")
	}
	if body.Detail == "" {
		t.Errorf("expected a detail string explaining why the state is unknown")
	}
}
