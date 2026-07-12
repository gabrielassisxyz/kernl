package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/tagname"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

// tagTreeDTO is one tag in the hierarchy the `/` convention implies.
//
// Count and ByType are subtree-inclusive and count distinct nodes: they are the
// size of the list a client gets by clicking the tag (which lists descendants
// too), so the number next to a tag and the page behind it can never disagree.
// A node carrying both `homelab` and `homelab/nas` is counted once under
// `homelab`.
type tagTreeDTO struct {
	Name     string         `json:"name"`
	Segment  string         `json:"segment"`
	Count    int            `json:"count"`
	ByType   map[string]int `json:"byType"`
	Children []tagTreeDTO   `json:"children"`
}

// taggedNodeDTO is one row of a tag's mixed-type result list.
//
// Path is populated for notes only: notes are file-backed and the Notes UI
// navigates by vault path, while every other type navigates by node id.
type taggedNodeDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	UpdatedAt string `json:"updatedAt"`
	Path      string `json:"path,omitempty"`
}

// RegisterTagRoutes exposes the tag surface, which is type-agnostic by design:
// a tag is the one axis that connects a note, a task and a bookmark that are
// about the same subject.
//
// The node listing is `/api/tags/nodes?tag=…` rather than `/api/tags/{name}/nodes`
// because tag names contain `/`, and ServeMux only allows a multi-segment
// wildcard as the final path element.
func RegisterTagRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/tags", listTagsHandler(a))
	mux.HandleFunc("GET /api/tags/nodes", tagNodesHandler(a))
}

// listTagsHandler answers "what tags exist", as a tree. System tags are hidden
// unless ?includeSystem=true — they are bookkeeping, not subjects the user chose.
func listTagsHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		includeSystem := r.URL.Query().Get("includeSystem") == "true"

		// Every (tag, node) pair, so counts can be aggregated up the tree over
		// distinct node ids rather than summed (which would double-count a node
		// carrying both a parent and its child).
		nodesByTag := map[string][]string{}
		typeByNode := map[string]string{}

		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(
				`SELECT t.name, n.id, n.type
				 FROM node_tags nt
				 JOIN tags t ON t.id = nt.tag_id
				 JOIN nodes n ON n.id = nt.node_id
				 WHERE n.deleted_at IS NULL`,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var name, nodeID, nodeType string
				if err := rows.Scan(&name, &nodeID, &nodeType); err != nil {
					return err
				}
				if !includeSystem && tags.IsSystem(name) {
					continue
				}
				nodesByTag[name] = append(nodesByTag[name], nodeID)
				typeByNode[nodeID] = nodeType
			}
			return rows.Err()
		})
		if err != nil {
			slog.Error("list tags", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list tags")
			return
		}

		names := make([]string, 0, len(nodesByTag))
		for name := range nodesByTag {
			names = append(names, name)
		}

		out := []tagTreeDTO{}
		for _, root := range tags.Tree(names) {
			dto, _ := tagSubtree(root, nodesByTag, typeByNode)
			out = append(out, dto)
		}
		writeJSON(w, out)
	}
}

// tagSubtree converts one tag and its descendants into DTOs, returning the set
// of distinct node ids the subtree covers so the parent can fold it into its own.
func tagSubtree(node *tags.TreeNode, nodesByTag map[string][]string, typeByNode map[string]string) (tagTreeDTO, map[string]struct{}) {
	covered := map[string]struct{}{}
	for _, id := range nodesByTag[node.Name] {
		covered[id] = struct{}{}
	}

	children := make([]tagTreeDTO, 0, len(node.Children))
	for _, child := range node.Children {
		childDTO, childNodes := tagSubtree(child, nodesByTag, typeByNode)
		children = append(children, childDTO)
		for id := range childNodes {
			covered[id] = struct{}{}
		}
	}

	byType := map[string]int{}
	for id := range covered {
		byType[typeByNode[id]]++
	}

	return tagTreeDTO{
		Name:     node.Name,
		Segment:  node.Segment,
		Count:    len(covered),
		ByType:   byType,
		Children: children,
	}, covered
}

// tagNodesHandler answers "what is under this tag", across every node type.
//
// This is a listing, not a search: it cannot go through search.Search, which
// requires a non-empty FTS query string.
func tagNodesHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		name, err := tagname.Normalize(query.Get("tag"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "tag is required and must be a valid tag name")
			return
		}
		// Descendants are the point of the nesting convention, so they are the
		// default; ?descendants=false narrows to the exact tag.
		descendants := query.Get("descendants") != "false"
		nodeType := strings.TrimSpace(query.Get("type"))

		out := []taggedNodeDTO{}
		err = a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			var nodeIDs []string
			var err error
			if descendants {
				nodeIDs, err = tags.NodesUnder(r.Context(), tx, name)
			} else {
				nodeIDs, err = tags.Nodes(r.Context(), tx, name)
			}
			if err != nil {
				return err
			}
			if len(nodeIDs) == 0 {
				return nil
			}

			placeholders := make([]string, len(nodeIDs))
			args := make([]any, 0, len(nodeIDs)+1)
			for i, id := range nodeIDs {
				placeholders[i] = "?"
				args = append(args, id)
			}
			typeFilter := ""
			if nodeType != "" {
				typeFilter = " AND n.type = ?"
				args = append(args, nodeType)
			}

			rows, err := tx.Query(
				`SELECT n.id, n.title, n.type, n.updated_at, np.path
				 FROM nodes n
				 LEFT JOIN note_paths np ON np.uuid = n.id
				 WHERE n.deleted_at IS NULL AND n.id IN (`+strings.Join(placeholders, ", ")+`)`+typeFilter+
					` ORDER BY n.updated_at DESC, n.id ASC`,
				args...,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var dto taggedNodeDTO
				var path sql.NullString
				if err := rows.Scan(&dto.ID, &dto.Title, &dto.Type, &dto.UpdatedAt, &path); err != nil {
					return err
				}
				dto.Path = path.String
				out = append(out, dto)
			}
			return rows.Err()
		})
		if err != nil {
			if errors.Is(err, tagname.ErrInvalid) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			slog.Error("list tag nodes", "tag", name, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list nodes for tag")
			return
		}

		// An unknown tag is an empty subject, not a missing page.
		writeJSON(w, out)
	}
}
