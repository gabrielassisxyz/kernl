package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func newTestAppWithGraph(t *testing.T) *app.App {
	t.Helper()
	g := testutil.NewInMemoryTestGraph(t)
	return &app.App{
		Graph:  g,
		Config: testCfg(),
	}
}

func TestDAIdentityGetReturnsDefaultOnFirstCall(t *testing.T) {
	a := newTestAppWithGraph(t)
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/da/identity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got nodes.DAIdentity
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.DisplayName != "Kernl Assistant" {
		t.Errorf("DisplayName = %q, want 'Kernl Assistant'", got.DisplayName)
	}
	if got.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}
	if got.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestDAIdentityGetIdempotent(t *testing.T) {
	a := newTestAppWithGraph(t)
	r := NewRouter(a)

	// First call creates.
	req1 := httptest.NewRequest("GET", "/api/da/identity", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	// Second call returns existing (no duplicate).
	req2 := httptest.NewRequest("GET", "/api/da/identity", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w1.Code != 200 || w2.Code != 200 {
		t.Fatalf("status: %d, %d", w1.Code, w2.Code)
	}

	var di1 nodes.DAIdentity
	json.Unmarshal(w1.Body.Bytes(), &di1)
	var di2 nodes.DAIdentity
	json.Unmarshal(w2.Body.Bytes(), &di2)

	if di1.ID != di2.ID {
		t.Errorf("expected same ID across calls, got %q vs %q", di1.ID, di2.ID)
	}

	// Verify only one row exists in DB.
	count := 0
	a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		rows, err := tx.Query(`SELECT COUNT(*) FROM nodes WHERE type = ?`, nodes.TypeDAIdentity)
		if err != nil {
			return err
		}
		defer rows.Close()
		if rows.Next() {
			rows.Scan(&count)
		}
		return nil
	})
	if count != 1 {
		t.Errorf("expected exactly 1 da_identity node, got %d", count)
	}
}

func TestDAIdentityPutUpdates(t *testing.T) {
	a := newTestAppWithGraph(t)

	// Seed via GET first.
	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/api/da/identity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// PUT update.
	body := `{"system_prompt":"New prompt","display_name":"New Name"}`
	putReq := httptest.NewRequest("PUT", "/api/da/identity", strings.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	r.ServeHTTP(putW, putReq)

	if putW.Code != 204 {
		t.Fatalf("PUT expected 204, got %d: %s", putW.Code, putW.Body.String())
	}

	// GET again to verify update.
	getReq := httptest.NewRequest("GET", "/api/da/identity", nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	if getW.Code != 200 {
		t.Fatalf("GET expected 200, got %d", getW.Code)
	}

	var got nodes.DAIdentity
	json.Unmarshal(getW.Body.Bytes(), &got)

	if got.SystemPrompt != "New prompt" {
		t.Errorf("SystemPrompt = %q, want 'New prompt'", got.SystemPrompt)
	}
	if got.DisplayName != "New Name" {
		t.Errorf("DisplayName = %q, want 'New Name'", got.DisplayName)
	}
}

func TestDAIdentityPutOnlyUpdatesProvidedFields(t *testing.T) {
	a := newTestAppWithGraph(t)
	r := NewRouter(a)

	// Seed via GET.
	req := httptest.NewRequest("GET", "/api/da/identity", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// PUT only display_name.
	body := `{"display_name":"Only Name Changed"}`
	putReq := httptest.NewRequest("PUT", "/api/da/identity", strings.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	r.ServeHTTP(putW, putReq)

	if putW.Code != 204 {
		t.Fatalf("PUT expected 204, got %d: %s", putW.Code, putW.Body.String())
	}

	// Verify system_prompt unchanged, display_name changed.
	getReq := httptest.NewRequest("GET", "/api/da/identity", nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	var got nodes.DAIdentity
	json.Unmarshal(getW.Body.Bytes(), &got)

	if got.DisplayName != "Only Name Changed" {
		t.Errorf("DisplayName = %q, want 'Only Name Changed'", got.DisplayName)
	}
	if !strings.Contains(got.SystemPrompt, "helpful assistant") {
		t.Errorf("SystemPrompt should still contain default, got %q", got.SystemPrompt)
	}
}

func TestDAIdentityConcurrentGetsNoDuplicate(t *testing.T) {
	a := newTestAppWithGraph(t)
	r := NewRouter(a)

	const n = 5
	type result struct {
		code int
		body []byte
	}
	ch := make(chan result, n)

	for i := 0; i < n; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/api/da/identity", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			ch <- result{code: w.Code, body: w.Body.Bytes()}
		}()
	}

	var firstID string
	for i := 0; i < n; i++ {
		res := <-ch
		if res.code != 200 {
			t.Errorf("concurrent request %d: status %d", i, res.code)
			continue
		}
		var di nodes.DAIdentity
		if err := json.Unmarshal(res.body, &di); err != nil {
			t.Errorf("concurrent request %d: unmarshal: %v", i, err)
			continue
		}
		if firstID == "" {
			firstID = di.ID
		} else if di.ID != firstID {
			t.Errorf("concurrent request %d: ID %q differs from first %q", i, di.ID, firstID)
		}
	}

	// Verify exactly one row.
	count := 0
	a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		rows, err := tx.Query(`SELECT COUNT(*) FROM nodes WHERE type = ?`, nodes.TypeDAIdentity)
		if err != nil {
			return err
		}
		defer rows.Close()
		if rows.Next() {
			rows.Scan(&count)
		}
		return nil
	})
	if count != 1 {
		t.Errorf("expected exactly 1 da_identity node, got %d", count)
	}
}
