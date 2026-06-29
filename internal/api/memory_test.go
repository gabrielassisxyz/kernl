package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/memory"
)

func setupMemoryTest(t *testing.T) (*app.App, *http.ServeMux) {
	g := testutil.NewInMemoryTestGraph(t)

	a := &app.App{Graph: g}
	mux := http.NewServeMux()
	RegisterMemoryRoutes(mux, a)

	// Add some data
	err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		_, err := nodes.CreateMemoryClaim(context.Background(), tx, nodes.MemoryClaim{
			Statement: "Go is strongly typed",
			Subject:   "go-programming",
		}, nodes.Author{Name: "system"})
		return err
	})
	if err != nil {
		t.Fatalf("failed to add claim: %v", err)
	}

	return a, mux
}

func TestMemoryAPI_Topics(t *testing.T) {
	_, mux := setupMemoryTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/memory/topics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %v: %s", w.Code, w.Body.String())
	}

	var res struct {
		Topics []string `json:"topics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(res.Topics) != 1 || res.Topics[0] != "go-programming" {
		t.Errorf("expected topic [go-programming], got %v", res.Topics)
	}
}

func TestMemoryAPI_Claims(t *testing.T) {
	_, mux := setupMemoryTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/memory/claims?topic=go-programming", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %v: %s", w.Code, w.Body.String())
	}

	var res struct {
		Claims []nodes.MemoryClaim `json:"claims"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(res.Claims) != 1 {
		t.Fatalf("expected 1 claim, got %v", len(res.Claims))
	}

	if res.Claims[0].Statement != "Go is strongly typed" {
		t.Errorf("unexpected statement: %v", res.Claims[0].Statement)
	}
}

func TestMemoryAPI_Telos(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Graph: g}
	mux := http.NewServeMux()
	RegisterMemoryRoutes(mux, a)

	// Two telos notes and one untagged note that must not surface.
	ctx := context.Background()
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{
			Title: "Who I am", Body: "I value leverage.", Tags: []string{"telos"},
		}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{
			Title: "Caching", Body: "LRU.", Tags: []string{"infra"},
		}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/memory/telos", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %v: %s", w.Code, w.Body.String())
	}

	var res struct {
		Notes []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"notes"`
		Injection struct {
			Bytes     int  `json:"bytes"`
			CapBytes  int  `json:"capBytes"`
			Truncated bool `json:"truncated"`
		} `json:"injection"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(res.Notes) != 1 {
		t.Fatalf("expected 1 telos note, got %d: %s", len(res.Notes), w.Body.String())
	}
	if res.Notes[0].Title != "Who I am" || res.Notes[0].Body != "I value leverage." {
		t.Errorf("unexpected telos note: %+v", res.Notes[0])
	}
	if res.Injection.CapBytes != 4000 {
		t.Errorf("capBytes = %d, want 4000", res.Injection.CapBytes)
	}
	if res.Injection.Bytes == 0 || res.Injection.Truncated {
		t.Errorf("expected a non-zero, non-truncated footprint, got %+v", res.Injection)
	}
}

func TestMemoryAPI_TelosEmpty(t *testing.T) {
	// No telos notes → empty list, zero footprint, still 200 with valid shape.
	_, mux := setupMemoryTest(t) // seeds only a claim, no telos note
	req := httptest.NewRequest(http.MethodGet, "/api/memory/telos", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %v: %s", w.Code, w.Body.String())
	}
	var res struct {
		Notes     []any `json:"notes"`
		Injection struct {
			Bytes int `json:"bytes"`
		} `json:"injection"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.Notes) != 0 {
		t.Errorf("expected no telos notes, got %d", len(res.Notes))
	}
	if res.Injection.Bytes != 0 {
		t.Errorf("expected zero footprint, got %d", res.Injection.Bytes)
	}
}

func TestMemoryAPI_Refute(t *testing.T) {
	a, mux := setupMemoryTest(t)

	// First get the claim ID
	var claimID string
	_ = a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		claims, err := memory.SynthesizeTopic(context.Background(), tx, "go-programming")
		if err != nil {
			return err
		}
		if len(claims) == 0 {
			t.Fatalf("no claims found")
		}
		claimID = claims[0].ID
		return nil
	})

	body := []byte(`{"reason":"not always"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/memory/claims/"+claimID+"/refute", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %v", w.Code)
	}

	// Verify claim is refuted
	req2 := httptest.NewRequest(http.MethodGet, "/api/memory/claims?topic=go-programming", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	var res struct {
		Claims []nodes.MemoryClaim `json:"claims"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &res)

	if len(res.Claims) != 0 {
		t.Errorf("expected 0 claims after refutation, got %v", len(res.Claims))
	}
}
