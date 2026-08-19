package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// seedZephyrNote gives Suggest something to offer: a note whose body carries a
// distinctive term the written note's body also carries.
func seedZephyrNote(t *testing.T, g *graph.Graph) {
	t.Helper()
	ctx := context.Background()
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateNote(ctx, tx, nodes.Note{
			Title: "Zephyr protocol",
			Body:  "The zephyr protocol handles retries and backoff.",
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed note: %v", err)
	}
}

func writeVaultFile(t *testing.T, a *app.App, channel string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	body := "---\ntitle: Zephyr notes\n---\n\nNotes about the zephyr protocol.\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path=zephyr-notes.md", bytes.NewBufferString(body))
	if channel != "" {
		req.Header.Set("X-Kernl-Client", channel)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// TestVaultWriteOffersSuggestionsByChannel verifies the gate: the assistant
// (cli) and unidentified clients get link suggestions, the web UI does not.
func TestVaultWriteOffersSuggestionsByChannel(t *testing.T) {
	cases := []struct {
		name    string
		channel string
		want    bool
	}{
		{"cli suggests", "cli", true},
		{"api suggests", "api", true},
		{"unidentified suggests", "", true},
		{"ui does not suggest", "ui", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			g := testutil.NewInMemoryTestGraph(t)
			seedZephyrNote(t, g)
			a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

			w := writeVaultFile(t, a, tc.channel)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var resp struct {
				Status      string                `json:"status"`
				Suggestions []nodes.LinkCandidate `json:"suggestions"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("response must be JSON: %v", err)
			}
			if resp.Status != "saved" {
				t.Errorf("status = %q, want %q", resp.Status, "saved")
			}
			if tc.want && len(resp.Suggestions) == 0 {
				t.Errorf("channel %q must offer suggestions, got none", tc.channel)
			}
			if !tc.want && len(resp.Suggestions) != 0 {
				t.Errorf("channel %q must not offer suggestions, got %d", tc.channel, len(resp.Suggestions))
			}
		})
	}
}
