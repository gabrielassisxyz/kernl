package chat

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func newPermissionTestApp(t *testing.T) *app.App {
	g := testutil.NewInMemoryTestGraph(t)
	return &app.App{
		Graph: g,
		Config: &config.Config{
			Vault: config.VaultConfig{Root: t.TempDir()},
		},
	}
}

// seedPolicyNote creates a note whose vault file path matches the given relative path
// and whose attrs carry that path so the global check can find it.
func seedPolicyNote(t *testing.T, a *app.App, title string, tags []string, vaultRelPath string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		n := nodes.Note{Title: title, Body: title, Tags: tags}
		id, err = nodes.CreateNote(ctx, tx, n, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		if vaultRelPath != "" {
			_, err = tx.Exec(`UPDATE nodes SET attrs=json_set(COALESCE(attrs,'{}'),'$.path',?) WHERE id=?`,
				vaultRelPath, id)
		}
		return err
	})
	if err != nil {
		t.Fatalf("seed policy note: %v", err)
	}
	return id
}

func writePolicyFile(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ".kernl-policies")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write .kernl-policies: %v", err)
	}
}

func TestGraphPermissionCheckerPublicNode(t *testing.T) {
	a := newPermissionTestApp(t)
	ctx := context.Background()
	var id string
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Public", Body: "b", Tags: []string{}}, nodes.Author{Name: "test"})
		return err
	})
	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected allowed, got denied: %s", reason)
	}
}

func TestGraphPermissionCheckerConfidentialTag(t *testing.T) {
	a := newPermissionTestApp(t)
	ctx := context.Background()
	var id string
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Secret", Body: "b", Tags: []string{"confidencial"}}, nodes.Author{Name: "test"})
		return err
	})
	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected denied")
	}
	if reason != "node marked private" {
		t.Errorf("reason = %q, want 'node marked private'", reason)
	}
}

func TestGraphPermissionCheckerNotFound(t *testing.T) {
	a := newPermissionTestApp(t)
	ctx := context.Background()
	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected denied")
	}
	if reason != "node not found" {
		t.Errorf("reason = %q, want 'node not found'", reason)
	}
}

func TestGraphPermissionCheckerGlobalPolicyDeny(t *testing.T) {
	vaultDir := t.TempDir()
	a := &app.App{
		Graph:  testutil.NewInMemoryTestGraph(t),
		Config: &config.Config{Vault: config.VaultConfig{Root: vaultDir}},
	}
	writePolicyFile(t, vaultDir, "*.secret.md")

	noteID := seedPolicyNote(t, a, "notes.secret.md", nil, "notes.secret.md")

	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(context.Background(), noteID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected denied by global policy *.secret.md")
	}
	if reason == "" {
		t.Error("expected non-empty denial reason from global policy")
	}
}

func TestGraphPermissionCheckerDigitalNodeSkipsGlobal(t *testing.T) {
	vaultDir := t.TempDir()
	a := &app.App{
		Graph:  testutil.NewInMemoryTestGraph(t),
		Config: &config.Config{Vault: config.VaultConfig{Root: vaultDir}},
	}
	// Write a policy that would deny everything — the node has no path, so it's skipped.
	writePolicyFile(t, vaultDir, "*.md")

	// Create a note with NO vault file path (digital-only, e.g. a chat message).
	ctx := context.Background()
	var id string
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Digital Note", Body: "body", Tags: []string{}}, nodes.Author{Name: "test"})
		return err
	})

	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected digital-only node to be allowed (no global check), got denied: %s", reason)
	}
}
