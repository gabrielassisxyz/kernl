package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

// taskDTO is the camelCase shape the web client consumes.
type taskDTO struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	ProjectID   string    `json:"projectId"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func RegisterTaskRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		listTasksHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		createTaskHandler(w, r, a)
	})
	mux.HandleFunc("PATCH /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		patchTaskHandler(w, r, a)
	})
}

func listTasksHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	projectID := r.URL.Query().Get("project")
	var out []taskDTO

	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		tasks, err := nodes.ListTasks(ctx, tx, projectID)
		if err != nil {
			return err
		}
		out = make([]taskDTO, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, taskDTO{
				ID:          t.ID,
				Title:       t.Title,
				Description: t.Description,
				Status:      t.Status,
				ProjectID:   t.ProjectID,
				Tags:        tagList(t.Tags),
				CreatedAt:   t.CreatedAt,
				UpdatedAt:   t.UpdatedAt,
			})
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func createTaskHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		ProjectID   string   `json:"projectId"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid task body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "task title is required")
		return
	}
	if err := tags.RejectSystem(req.Tags); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	author := nodes.Author{Name: "api"}
	title := strings.TrimSpace(req.Title)
	var id string
	var companion CompanionFile
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateTask(ctx, tx, nodes.Task{
			Title:       title,
			Description: req.Description,
			Status:      req.Status,
			ProjectID:   req.ProjectID,
			Tags:        req.Tags,
		}, author)
		if err != nil {
			return err
		}
		// Canonical graph relationship: task -[part_of]-> project.
		if req.ProjectID != "" {
			if _, err = edges.Create(ctx, tx, edges.Edge{
				Src:  id,
				Dst:  req.ProjectID,
				Type: edges.EdgeTypePartOf,
			}, author); err != nil {
				return err
			}
		}
		companion, err = CreateCompanionNote(ctx, tx, a, id, "tasks", title)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task: "+err.Error())
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

func patchTaskHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing task id")
		return
	}
	// Pointer fields distinguish "absent" from "set to empty": an omitted tags
	// key leaves the task's tags alone, while `"tags": []` clears them.
	var req struct {
		Status *string   `json:"status"`
		Tags   *[]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid patch body: "+err.Error())
		return
	}
	if req.Status == nil && req.Tags == nil {
		writeError(w, http.StatusBadRequest, "nothing to update: provide status or tags")
		return
	}
	if req.Status != nil && *req.Status == "" {
		writeError(w, http.StatusBadRequest, "status cannot be empty")
		return
	}
	if req.Tags != nil {
		if err := tags.RejectSystem(*req.Tags); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	ctx := r.Context()
	author := nodes.Author{Name: "api"}
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if req.Status != nil {
			if err := nodes.SetTaskStatus(ctx, tx, id, *req.Status, author); err != nil {
				return err
			}
		}
		if req.Tags != nil {
			return nodes.SetTaskTags(ctx, tx, id, *req.Tags, author)
		}
		return nil
	})
	if err == graph.ErrNotFound {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update task: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tagList normalises a possibly-nil tag slice into a JSON array, so clients can
// treat tags as an array unconditionally rather than guarding against null.
func tagList(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
