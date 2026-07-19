package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The approval routes must not fake a working gate. Until the capture flow is
// built, every approval route answers 501 with an honest message, so a caller
// can tell "unbuilt" from "idle" and never acts on a fabricated id. See the
// package comment in approvals.go.
func TestApprovalRoutesReportNotImplemented(t *testing.T) {
	r := NewRouter(testApp())

	cases := []struct {
		method, path string
	}{
		{"GET", "/api/approvals"},
		{"POST", "/api/approvals/apr-999/actions"},
		{"POST", "/api/terminal/sess-1/approvals/apr-999"},
	}

	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotImplemented {
			t.Errorf("%s %s: want 501, got %d (body %q)", c.method, c.path, w.Code, w.Body.String())
			continue
		}
		var body struct {
			Error       string `json:"error"`
			Implemented bool   `json:"implemented"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %s: body is not JSON: %v", c.method, c.path, err)
			continue
		}
		if body.Implemented {
			t.Errorf("%s %s: must report implemented=false", c.method, c.path)
		}
		if !strings.Contains(body.Error, "not implemented") {
			t.Errorf("%s %s: error must say it is not implemented, got %q", c.method, c.path, body.Error)
		}
	}
}
