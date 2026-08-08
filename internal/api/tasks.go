package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// taskDTO is the camelCase shape the web client consumes.
type taskDTO struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	ProjectID   string   `json:"projectId"`
	Tags        []string `json:"tags"`
	// DueDate is a calendar day (YYYY-MM-DD), empty when the task has none  -
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
	var companionFile companion.File
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
		companionFile, err = companion.Create(ctx, tx, a.Config.Vault.Root, id, layout.TasksFolder, title, req.Description, "task")
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task: "+err.Error())
		return
	}
	if err := companion.WriteFile(a.Config.Vault.Root, companionFile); err != nil {
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
	// dueDate - `"dueDate": ""` is how a due date is removed.
	var req struct {
		Title       *string   `json:"title"`
		Description *string   `json:"description"`
		Status      *string   `json:"status"`
		ProjectID   *string   `json:"projectId"`
		Tags        *[]string `json:"tags"`
		DueDate     *string   `json:"dueDate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid patch body: "+err.Error())
		return
	}
	if req.Title == nil && req.Description == nil && req.Status == nil && req.ProjectID == nil && req.Tags == nil && req.DueDate == nil {
		writeError(w, http.StatusBadRequest, "nothing to update: provide title, description, status, projectId, tags or dueDate")
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
	var companionFile companion.File
	// A project id that names nothing would leave the task pointing at a
	// project the UI cannot resolve, so it is rejected rather than stored. The
	// check lives inside the transaction because that is where the write is.
	var unknownProject bool
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if req.ProjectID != nil && *req.ProjectID != "" {
			var one int
			switch err := tx.QueryRow(
				`SELECT 1 FROM nodes WHERE id = ? AND type = 'project' AND deleted_at IS NULL`,
				*req.ProjectID,
			).Scan(&one); {
			case err == sql.ErrNoRows:
				unknownProject = true
				return graph.ErrNotFound
			case err != nil:
				return err
			}
		}
		if req.Title != nil {
			// The companion note keeps its file name. After duplicate titles
			// became legal the path stopped being a function of the title, so a
			// rename would have to guess which file belongs to this task instead
			// of asking the describes edge.
			if err := nodes.SetTaskTitle(ctx, tx, id, strings.TrimSpace(*req.Title), author); err != nil {
				return err
			}
		}
		if req.Description != nil {
			if err := nodes.SetTaskDescription(ctx, tx, id, *req.Description, author); err != nil {
				return err
			}
			var err error
			if companionFile, err = companion.SyncDescription(ctx, tx, a.Config.Vault.Root, id, *req.Description); err != nil {
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
		if req.ProjectID != nil {
			if err := nodes.SetTaskProject(ctx, tx, id, *req.ProjectID, author); err != nil {
				return err
			}
			// The attr above is the filtering mirror; this is the canonical
			// link. Both move together or a reassigned task keeps answering the
			// old project's graph queries.
			if _, err := edges.DeleteBySource(ctx, tx, id, edges.EdgeTypePartOf, author); err != nil {
				return err
			}
			if *req.ProjectID != "" {
				if _, err := edges.Create(ctx, tx, edges.Edge{
					Src:  id,
					Dst:  *req.ProjectID,
					Type: edges.EdgeTypePartOf,
				}, author); err != nil {
					return err
				}
			}
		}
		if req.Tags != nil {
			return nodes.SetTaskTags(ctx, tx, id, *req.Tags, author)
		}
		return nil
	})
	if unknownProject {
		writeError(w, http.StatusBadRequest, "projectId does not name an existing project")
		return
	}
	if err == graph.ErrNotFound {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update task: "+err.Error())
		return
	}
	// After the commit, mirroring the create path: the hash the transaction
	// recorded describes these bytes, so the file is written last.
	if err := companion.WriteFile(a.Config.Vault.Root, companionFile); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update companion note: "+err.Error())
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
	// tx; the file afterwards). A task has no children to cascade - unlike a
	// project, nothing points at it that we own.
	var companionPath string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		if companionPath, err = companion.Delete(ctx, tx, id); err != nil {
			return err
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

	companion.RemoveFile(a.Config.Vault.Root, companionPath)
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
