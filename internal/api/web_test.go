package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerServesWebIndex(t *testing.T) {
	r := NewRouter(testApp())
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Kernl") {
		t.Errorf("expected web/index.html at /, got %d %q", w.Code, w.Body.String()[:80])
	}
}
