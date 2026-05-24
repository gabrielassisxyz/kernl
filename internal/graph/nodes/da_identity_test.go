package nodes

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestDAIdentityRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	di := &DAIdentity{
		SystemPrompt: "You are a helpful assistant.",
		DisplayName:  "Kernl Assistant",
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateDAIdentity(ctx, tx, di, Author{Name: "kernl"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateDAIdentity: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *DAIdentity
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetDAIdentity(ctx, tx)
		return err
	})
	if err != nil {
		t.Fatalf("GetDAIdentity: %v", err)
	}

	if got.ID != id {
		t.Errorf("ID = %q, want %q", got.ID, id)
	}
	if got.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("SystemPrompt = %q, want 'You are a helpful assistant.'", got.SystemPrompt)
	}
	if got.DisplayName != "Kernl Assistant" {
		t.Errorf("DisplayName = %q, want 'Kernl Assistant'", got.DisplayName)
	}
}

func TestDAIdentityFTSFields(t *testing.T) {
	di := &DAIdentity{
		ID:           "id",
		DisplayName:  "Assistant",
		SystemPrompt: "Short",
	}
	fts := di.FTSFields()
	if fts.Title != "Assistant" {
		t.Errorf("Title = %q, want Assistant", fts.Title)
	}
	if fts.Body != "Short" {
		t.Errorf("Body = %q, want Short", fts.Body)
	}
}

func TestDAIdentityFTSFieldsTruncated(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	di := &DAIdentity{
		ID:           "id",
		DisplayName:  "A",
		SystemPrompt: string(long),
	}
	fts := di.FTSFields()
	if len(fts.Body) != 200 {
		t.Errorf("Body len = %d, want 200", len(fts.Body))
	}
}

func TestDAIdentityUpdate(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	di := &DAIdentity{
		SystemPrompt: "First",
		DisplayName:  "First Name",
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateDAIdentity(ctx, tx, di, Author{Name: "kernl"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateDAIdentity: %v", err)
	}

	updated := &DAIdentity{
		ID:           id,
		SystemPrompt: "Updated",
		DisplayName:  "Updated Name",
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SaveDAIdentity(ctx, tx, updated, Author{Name: "kernl"})
	})
	if err != nil {
		t.Fatalf("SaveDAIdentity: %v", err)
	}

	var got *DAIdentity
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetDAIdentity(ctx, tx)
		return err
	})
	if err != nil {
		t.Fatalf("GetDAIdentity after save: %v", err)
	}
	if got.SystemPrompt != "Updated" {
		t.Errorf("SystemPrompt = %q, want Updated", got.SystemPrompt)
	}
	if got.DisplayName != "Updated Name" {
		t.Errorf("DisplayName = %q, want Updated Name", got.DisplayName)
	}
}

func TestDAIdentityGetNotFound(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		_, err := GetDAIdentity(ctx, tx)
		return err
	})
	if err != graph.ErrNotFound {
		t.Fatalf("expected graph.ErrNotFound, got %v", err)
	}
}
