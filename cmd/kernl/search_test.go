package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// TestSearchAndPlanShareOneDispatchFunction is the alias's whole contract: both
// verbs must route through the same function variable, never two copies of the
// implementation. Stubbing the one variable and watching BOTH verbs follow it
// is what catches a copied implementation - a separate `searchFn` would keep
// running the real graph-opening code while `plan` hits this stub.
func TestSearchAndPlanShareOneDispatchFunction(t *testing.T) {
	reached := false
	orig := planFn
	planFn = func(string, []string) error { reached = true; return nil }
	t.Cleanup(func() { planFn = orig })

	for _, verb := range []string{"search", "plan"} {
		reached = false
		if err := Dispatch([]string{verb, "some topic"}); err != nil {
			t.Fatalf("Dispatch(%q) = %v", verb, err)
		}
		if !reached {
			t.Errorf("Dispatch(%q) did not reach the shared plan function", verb)
		}
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what it
// wrote, failing the test if fn returns an error.
func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	return string(out)
}

// TestSearchReturnsResultsWithoutAServer pins the serverless property of the
// retrieval verb. No server is started anywhere in this test: the graph is
// seeded on disk, then kernl search / kernl plan open it in process. If the
// verb were swapped onto the HTTP path later, it would return a connection
// error here instead of the seeded note - the one failure the scorer exists to
// avoid. The byte-identical check rides on the same run because both verbs
// dispatch to the same function.
func TestSearchReturnsResultsWithoutAServer(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	dbPath := filepath.Join(dir, ".kernl-graph.db")

	cfgContent := `settings:
  agents:
    dummy:
      command: dummy
registry:
  repos:
    - path: ` + dir + `
      memoryManager: br
vault:
  root: ` + dir + `
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	err = g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		_, err := nodes.CreateNote(context.Background(), tx, nodes.Note{
			Title: "Quicksilver Cache",
			Body:  "The cache invalidation strategy uses quicksilver tokens to bust stale entries.",
		}, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		g.Close()
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}

	outputs := map[string]string{}
	for _, verb := range []string{"search", "plan"} {
		out := captureStdout(t, func() error {
			return Dispatch([]string{"--config", cfgPath, verb, "quicksilver"})
		})
		if !strings.Contains(out, "Quicksilver Cache") {
			t.Errorf("%s must return the seeded note with no server running, got: %q", verb, out)
		}
		outputs[verb] = out
	}
	if outputs["search"] != outputs["plan"] {
		t.Errorf("search and plan must produce byte-identical output:\nsearch: %q\nplan:   %q",
			outputs["search"], outputs["plan"])
	}
}
