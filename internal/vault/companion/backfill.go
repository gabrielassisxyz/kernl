package companion

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// querier is the one method Missing needs, satisfied by both graph.ReadTx and
// graph.WriteTx. The scan has two callers that cannot share a transaction type:
// the doctor check reads, and the backfill has to see the same list from inside
// the write transaction that repairs it, or it would repair a list that another
// writer has already changed.
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// Orphan is an entity that has no companion note.
//
// It carries what Create needs to build one, read from the node rather than from
// the caller: an entity that lost its companion is exactly the case where no
// caller has the title and description to hand.
type Orphan struct {
	ID          string
	Type        string // task, project or bookmark
	Title       string
	Description string
	Folder      string // where its companion belongs, from the vault layout
}

// entityFolders maps the node types that own a companion to the layout folder
// theirs belongs in. A type absent here is a type with no companion by design,
// so the map is also the definition of "which entities this sweep covers".
var entityFolders = map[string]string{
	"task":     layout.TasksFolder,
	"project":  layout.ProjectsFolder,
	"bookmark": layout.BookmarksFolder,
}

// Missing lists live entities with no companion note, oldest first.
//
// A node counts as missing only when NO live note describes it, which is the
// same describes edge Create writes and noteFor reads. It deliberately does not
// check the filesystem: an entity whose note node exists but whose file was
// deleted is a different condition (the reconciler's), and conflating the two
// would have this sweep write a file that a note_paths row already claims.
func Missing(ctx context.Context, tx querier) ([]Orphan, error) {
	// NOT EXISTS, not a LEFT JOIN with a NULL test. The join asks the question per
	// EDGE and this one has to be asked per ENTITY: an entity that lost a companion
	// once and has a live one now carries two describes edges, the join emits a row
	// for each, and the row for the dead one has a NULL companion that passes a
	// "no companion" filter. That false positive made the sweep offer to write a
	// second companion for an entity that already had one, so re-running it would
	// have duplicated exactly what it exists to repair.
	rows, err := tx.Query(`
		SELECT n.id, n.type, n.title, n.attrs
		FROM nodes n
		WHERE n.type IN ('task', 'project', 'bookmark')
		  AND n.deleted_at IS NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM edges e
		    JOIN nodes cn ON cn.id = e.src AND cn.type = 'note' AND cn.deleted_at IS NULL
		    WHERE e.dst = n.id AND e.label = ?
		  )
		ORDER BY n.created_at`, EdgeLabel)
	if err != nil {
		return nil, fmt.Errorf("companion: scan for missing companions: %w", err)
	}
	defer rows.Close()

	var out []Orphan
	for rows.Next() {
		var o Orphan
		var attrs string
		if err := rows.Scan(&o.ID, &o.Type, &o.Title, &attrs); err != nil {
			return nil, fmt.Errorf("companion: read entity row: %w", err)
		}
		o.Folder = entityFolders[o.Type]
		o.Description = descriptionFrom(attrs)
		out = append(out, o)
	}
	return out, rows.Err()
}

// descriptionFrom pulls the description out of a node's attrs blob. A node whose
// attrs have no description, or whose attrs do not parse, yields "" - the same
// value Create takes to mean "do not stamp a description", so unreadable attrs
// cost a frontmatter line rather than the whole backfill.
func descriptionFrom(attrs string) string {
	var parsed struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(attrs), &parsed); err != nil {
		return ""
	}
	return parsed.Description
}

// Backfill writes the missing companion note for every entity in orphans and
// returns the files for the caller to write once the transaction has committed.
//
// It is the repair half of Missing, and it is deliberately NOT automatic. The
// sweep cannot tell an entity that never had a companion from one whose note the
// user deleted on purpose, and SyncDescription already refuses to resurrect the
// second kind ("a companion whose file the user deleted stays deleted"). So the
// decision to recreate belongs to whoever is asking, and the caller is expected
// to have confirmed it.
func Backfill(ctx context.Context, tx *graph.WriteTx, vaultRoot string, orphans []Orphan) ([]File, error) {
	files := make([]File, 0, len(orphans))
	for _, o := range orphans {
		folder, ok := entityFolders[o.Type]
		if !ok {
			// Not reachable through Missing, which filters by the same map. Loud
			// rather than skipped: it would mean the two have drifted apart.
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: node type %q owns no companion folder - add it to entityFolders in internal/vault/companion/backfill.go", o.Type)
		}
		cf, err := Create(ctx, tx, vaultRoot, o.ID, folder, o.Title, o.Description, o.Type)
		if err != nil {
			return nil, err
		}
		files = append(files, cf)
	}
	return files, nil
}
