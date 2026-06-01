package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

type mockExtractor struct {
	actions []Action
	err     error
}

func (m *mockExtractor) ExtractActions(ctx context.Context, content string) ([]Action, error) {
	return m.actions, m.err
}

func TestServiceProcessFile(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	tempDir := t.TempDir()
	mm := NewManifestManager(tempDir)
	_ = mm.Load()

	mockEx := &mockExtractor{
		actions: []Action{
			{Type: "Create Page", Title: "New Concept", Payload: "some data"},
			{Type: "Contradiction Callout", Title: "Conflict found", Payload: "diff data"},
		},
	}

	svc := NewService(g, mm, mockEx)

	testFile := filepath.Join(tempDir, "test.md")
	content := []byte("test content")
	_ = os.WriteFile(testFile, content, 0644)

	err := svc.ProcessFile(context.Background(), testFile, "node123")
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}

	// Verify reviews were created
	err = g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		reviews, err := nodes.ListIngestReviews(context.Background(), tx, nodes.IngestReviewFilter{})
		if err != nil {
			return err
		}
		if len(reviews) != 2 {
			t.Errorf("Expected 2 reviews, got %d", len(reviews))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify manifest updated
	if mm.NeedsProcessing(testFile, content) {
		t.Error("Expected file to be marked as processed")
	}

	// Processing again should do nothing and not create more reviews
	err = svc.ProcessFile(context.Background(), testFile, "node123")
	if err != nil {
		t.Fatalf("ProcessFile second time failed: %v", err)
	}

	err = g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		reviews, err := nodes.ListIngestReviews(context.Background(), tx, nodes.IngestReviewFilter{})
		if err != nil {
			return err
		}
		if len(reviews) != 2 {
			t.Errorf("Expected 2 reviews after second process, got %d", len(reviews))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
