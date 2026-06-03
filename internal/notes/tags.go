package notes

import (
	"context"
	"sort"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// TagNode is one tag in the hierarchy with the note paths carrying it.
type TagNode struct {
	Files []string `json:"files"`
}

// TagTree groups note file paths by tag, sourced from the graph (node_tags +
// note_paths) so the editor's tag pane needs a single request instead of one
// fetch per file.
func TagTree(ctx context.Context, g *graph.Graph) (map[string]TagNode, error) {
	byTag := map[string][]string{}
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		rows, err := tx.Query(`
			SELECT tg.name, np.path
			FROM node_tags nt
			JOIN tags tg ON tg.id = nt.tag_id
			JOIN note_paths np ON np.uuid = nt.node_id
			JOIN nodes n ON n.id = nt.node_id
			WHERE n.type = 'note' AND n.deleted_at IS NULL
			ORDER BY tg.name, np.path`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tag, path string
			if err := rows.Scan(&tag, &path); err != nil {
				return err
			}
			byTag[tag] = append(byTag[tag], path)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	out := make(map[string]TagNode, len(byTag))
	for tag, files := range byTag {
		sort.Strings(files)
		out[tag] = TagNode{Files: files}
	}
	return out, nil
}
