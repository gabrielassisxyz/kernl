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
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// taskDTO is the camelCase shape the web client consumes.
type taskDTO struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	ProjectID   string   `json:"projectId"`
	Tags        []string `json:"tags"`
	// DueDate is a calendar day (YYYY-MM-DD), empty when the task has none —
	// deliberately not an RFC3339 timestamp like the two below: a due date
	// rendered through a timezone is a due date that shows up a day early.
	DueDate   string    `json:"dueDate"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
	mux.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteTaskHandler(w, r, a)
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
				DueDate:     nodes.FormatDueDate(t.DueDate),
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
		DueDate     string   `json:"dueDate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid task body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "task title is required")
		return
	}
	dueDate, err := nodes.ParseDueDate(req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	author := nodes.Author{Name: "api"}
	title := strings.TrimSpace(req.Title)
	var id string
	var companion CompanionFile
	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateTask(ctx, tx, nodes.Task{
			Title:       title,
			Description: req.Description,
			Status:      req.Status,
			ProjectID:   req.ProjectID,
			Tags:        req.Tags,
			DueDate:     dueDate,
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
		companion, err = CreateCompanionNote(ctx, tx, a, id, "tasks", title, "task")
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
	// key leaves the task's tags alone, while `"tags": []` clears them. Same for
	// dueDate — `"dueDate": ""` is how a due date is removed.
	var req struct {
		Title   *string   `json:"title"`
		Status  *string   `json:"status"`
		Tags    *[]string `json:"tags"`
		DueDate *string   `json:"dueDate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid patch body: "+err.Error())
		return
	}
	if req.Title == nil && req.Status == nil && req.Tags == nil && req.DueDate == nil {
		writeError(w, http.StatusBadRequest, "nothing to update: provide title, status, tags or dueDate")
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
	var dueDate *time.Time
	if req.DueDate != nil {
		var err error
		if dueDate, err = nodes.ParseDueDate(*req.DueDate); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	ctx := r.Context()
	author := nodes.Author{Name: "api"}
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if req.Title != nil {
			if err := nodes.SetTaskTitle(ctx, tx, id, strings.TrimSpace(*req.Title), author); err != nil {
				return err
			}
		}
		if req.Status != nil {
			if err := nodes.SetTaskStatus(ctx, tx, id, *req.Status, author); err != nil {
				return err
			}
		}
		if req.DueDate != nil {
			if err := nodes.SetTaskDueDate(ctx, tx, id, dueDate, author); err != nil {
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

func deleteTaskHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing task id")
		return
	}

	ctx := r.Context()
	// The companion note goes with the task (node + note_paths row in the same
	// tx; the file afterwards). A task has no children to cascade — unlike a
	// project, nothing points at it that we own.
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
		return nodes.DeleteTask(ctx, tx, id, nodes.Author{Name: "api"})
	})
	if err == graph.ErrNotFound {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete task: "+err.Error())
		return
	}

	// Best-effort file removal after the transaction committed; the watcher
	// treats a delete event for an already-gone node as a no-op.
	if companionPath != "" && a.Config.Vault.Root != "" {
		_ = os.Remove(filepath.Join(a.Config.Vault.Root, filepath.FromSlash(companionPath)))
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
