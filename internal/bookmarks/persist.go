package bookmarks

import (
	"context"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// ArchiveAndPersist archives a bookmark's URL (raw HTML + excerpt) and writes
// the updated node back to the graph. Safe to run in a background goroutine so
// the API can return immediately while archiving proceeds.
func ArchiveAndPersist(ctx context.Context, g *graph.Graph, archiver *Archiver, id string) error {
	var b *nodes.Bookmark
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		return err
	}

	if _, err := archiver.ArchiveBookmark(ctx, b); err != nil {
		return err
	}

	return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateBookmark(ctx, tx, *b, nodes.Author{Name: "archiver"})
	})
}
