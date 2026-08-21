package wikilink

import (
	"context"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// ResolveDescriptionInTx turns the wikilinks in a task's or project's description
// into links_to edges, exactly as a note's body gets them.
//
// WHY IT IS A SEPARATE ENTRY POINT rather than a call the reconciler makes. Wikilink
// resolution is driven by FILE CHANGE: the reconciler reads a note off disk and hands
// the body to the resolver. A description is a field on a node, inside a JSON blob in
// attrs, and no file changes when it does. So the trigger has to live where the field
// is written, which is the entity's own write path, and there is no file event to hook.
//
// The description IS mirrored into the companion note's frontmatter, which is a file -
// but the reconciler hands the resolver the body BELOW the frontmatter, so a wikilink
// written in a description was parsed by nothing. Writing it into the companion's body
// instead would duplicate in a file what the node already holds, to work around a
// trigger that does not fire.
//
// The edge is sourced from the ENTITY, not from its companion note: the task is what
// says it relates to the target. That direction is also what makes this safe, because
// resolveInTx clears every links_to edge leaving the source before it rebuilds them,
// and nothing else in kernl creates a links_to edge OUT of a task or a project - the
// companion note points at its entity, not the other way round.
//
// Dangling works unchanged: a description naming a note that does not exist yet parks a
// row keyed to the entity, and adoption promotes it into an entity-to-note edge.
func ResolveDescriptionInTx(ctx context.Context, tx *graph.WriteTx, nodeID, description string) error {
	_, err := resolveInTx(ctx, tx, nodeID, description)
	return err
}
