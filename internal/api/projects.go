package api

import (
	"encoding/json"
	"net/http"
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
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid project body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "project title is required")
		return
	}

	ctx := r.Context()
	var id string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateProject(ctx, tx, nodes.Project{
			Title:       strings.TrimSpace(req.Title),
			Description: req.Description,
			Status:      req.Status,
		}, nodes.Author{Name: "api"})
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project: "+err.Error())
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
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid patch body: "+err.Error())
		return
	}
	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}

	ctx := r.Context()
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetProjectStatus(ctx, tx, id, req.Status, nodes.Author{Name: "api"})
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
