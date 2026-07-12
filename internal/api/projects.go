package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// projectDTO is the camelCase shape the web client consumes.
type projectDTO struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	TaskCount   int       `json:"taskCount"`
	DoneCount   int       `json:"doneCount"`
}

func RegisterProjectRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		listProjectsHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		createProjectHandler(w, r, a)
	})
	mux.HandleFunc("PATCH /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		patchProjectHandler(w, r, a)
	})
	mux.HandleFunc("DELETE /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteProjectHandler(w, r, a)
	})
}

func listProjectsHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	var out []projectDTO

	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		projects, err := nodes.ListProjects(ctx, tx)
		if err != nil {
			return err
		}

		// Per-project task counts in one pass (no N+1).
		total := map[string]int{}
		done := map[string]int{}
		rows, err := tx.Query(
			`SELECT json_extract(attrs, '$.projectId') AS pid, json_extract(attrs, '$.status') AS st
			 FROM nodes WHERE type = 'task' AND deleted_at IS NULL`,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var pid, st *string
			if err := rows.Scan(&pid, &st); err != nil {
				return err
			}
			if pid == nil || *pid == "" {
				continue
			}
			total[*pid]++
			if st != nil && *st == nodes.TaskStatusDone {
				done[*pid]++
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}

		out = make([]projectDTO, 0, len(projects))
		for _, p := range projects {
			out = append(out, projectDTO{
				ID:          p.ID,
				Title:       p.Title,
				Description: p.Description,
				Status:      p.Status,
				Tags:        tagList(p.Tags),
				CreatedAt:   p.CreatedAt,
				UpdatedAt:   p.UpdatedAt,
				TaskCount:   total[p.ID],
				DoneCount:   done[p.ID],
			})
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list projects: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func createProjectHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid project body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "project title is required")
		return
	}
	if err := validateUserTags(req.Tags); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	title := strings.TrimSpace(req.Title)
	var id string
	var companion CompanionFile
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateProject(ctx, tx, nodes.Project{
			Title:       title,
			Description: req.Description,
			Status:      req.Status,
			Tags:        req.Tags,
		}, nodes.Author{Name: "api"})
		if err != nil {
			return err
		}
		companion, err = CreateCompanionNote(ctx, tx, a, id, "projects", title)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project: "+err.Error())
		return
	}
	if err := WriteCompanionFile(a, companion); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write companion note: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func patchProjectHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}
	// Pointer fields distinguish "absent" from "set to empty" — clearing the
	// description or the tag list is a legitimate edit, dropping the title is not.
	var req struct {
		Title       *string   `json:"title"`
		Description *string   `json:"description"`
		Status      *string   `json:"status"`
		Tags        *[]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid patch body: "+err.Error())
		return
	}
	if req.Title == nil && req.Description == nil && req.Status == nil && req.Tags == nil {
		writeError(w, http.StatusBadRequest, "nothing to update: provide title, description, status, or tags")
		return
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title cannot be empty")
		return
	}
	if req.Status != nil && *req.Status == "" {
		writeError(w, http.StatusBadRequest, "status cannot be empty")
		return
	}
	if req.Tags != nil {
		if err := validateUserTags(*req.Tags); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	ctx := r.Context()
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if req.Title != nil || req.Description != nil {
			// Read-modify-write inside the same tx so a partial patch (title
			// only, or description only) preserves the other field.
			var title, attrsRaw, createdAt, updatedAt sql.NullString
			err := tx.QueryRow(
				`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'project' AND deleted_at IS NULL`,
				id,
			).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
			if err == sql.ErrNoRows {
				return graph.ErrNotFound
			}
			if err != nil {
				return err
			}
			var attrs struct {
				Description string `json:"description"`
			}
			if attrsRaw.Valid && attrsRaw.String != "" {
				_ = json.Unmarshal([]byte(attrsRaw.String), &attrs)
			}
			newTitle := title.String
			newDescription := attrs.Description
			if req.Title != nil {
				newTitle = strings.TrimSpace(*req.Title)
			}
			if req.Description != nil {
				newDescription = *req.Description
			}
			if err := nodes.UpdateProjectMeta(ctx, tx, id, newTitle, newDescription, nodes.Author{Name: "api"}); err != nil {
				return err
			}
		}
		if req.Status != nil {
			if err := nodes.SetProjectStatus(ctx, tx, id, *req.Status, nodes.Author{Name: "api"}); err != nil {
				return err
			}
		}
		if req.Tags != nil {
			return nodes.SetProjectTags(ctx, tx, id, *req.Tags, nodes.Author{Name: "api"})
		}
		return nil
	})
	if err == graph.ErrNotFound {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update project: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func deleteProjectHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}

	ctx := r.Context()
	// The companion note goes with the project (node + note_paths row in the
	// same tx; the file afterwards). Tasks are NOT cascaded — they keep their
	// projectId attr and simply render as unassigned.
	var companionPath string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var noteID string
		err := tx.QueryRow(
			`SELECT e.src FROM edges e
			 JOIN nodes n ON n.id = e.src AND n.type = 'note' AND n.deleted_at IS NULL
			 WHERE e.dst = ? AND e.label = ?`,
			id, companionEdgeLabel,
		).Scan(&noteID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if noteID != "" {
			_ = tx.QueryRow(`SELECT path FROM note_paths WHERE uuid = ?`, noteID).Scan(&companionPath)
			if err := nodes.DeleteNote(ctx, tx, noteID, nodes.Author{Name: "api"}); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM note_paths WHERE uuid = ?`, noteID); err != nil {
				return err
			}
		}
		return nodes.DeleteProject(ctx, tx, id, nodes.Author{Name: "api"})
	})
	if err == graph.ErrNotFound {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete project: "+err.Error())
		return
	}

	// Best-effort file removal after the transaction committed; the watcher
	// treats a delete event for an already-gone node as a no-op.
	if companionPath != "" && a.Config.Vault.Root != "" {
		_ = os.Remove(filepath.Join(a.Config.Vault.Root, filepath.FromSlash(companionPath)))
	}
	w.WriteHeader(http.StatusNoContent)
}
