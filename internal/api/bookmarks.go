package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// startBookmarkArchive keeps page fetching off the create request's critical
// path. Tests replace it with a synchronous recorder so they do not leave a
// writer racing the temporary vault's cleanup.
var startBookmarkArchive = func(g *graph.Graph, vaultRoot, id string) {
	archiver := bookmarks.NewArchiver(nil, bookmarks.ArchiveDir(vaultRoot))
	go func() {
		if err := bookmarks.ArchiveAndPersist(context.Background(), g, archiver, id); err != nil {
			slog.Warn("bookmark archive failed", "id", id, "error", err)
		}
	}()
}

func RegisterBookmarkRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("POST /api/bookmarks", func(w http.ResponseWriter, r *http.Request) {
		createBookmarkHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/bookmarks", func(w http.ResponseWriter, r *http.Request) {
		listBookmarksHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/bookmarks/{id}", func(w http.ResponseWriter, r *http.Request) {
		getBookmarkHandler(w, r, a)
	})
	mux.HandleFunc("PATCH /api/bookmarks/{id}", func(w http.ResponseWriter, r *http.Request) {
		patchBookmarkHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/bookmarks/{id}/highlights", func(w http.ResponseWriter, r *http.Request) {
		addHighlightHandler(w, r, a)
	})
}

func createBookmarkHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	var req bookmarkCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var id string
	var companionFile companion.File

	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "api"}
		// The URL stands in as the title until the archiver extracts the real
		// one. It is a poor title but a true one, which a placeholder word is
		// not: "Pending" outlived every bookmark that ever carried it, because
		// nothing downstream could tell it apart from a title someone meant.
		b := nodes.Bookmark{URL: req.URL, Title: req.URL, Tags: req.Tags}

		var err error
		id, err = nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return err
		}
		// Named after the bookmark's title, the one rule for every companion:
		// a task's note is named after the task, and a bookmark's after the
		// bookmark. Here that resolves to the URL, because archiving is
		// asynchronous and b.Title is still standing in for the real one - the
		// same output the URL used to be hardcoded to, now as a consequence of
		// the rule rather than an exception to it. The name does not change when
		// the archiver later learns the title: the note's file stem is its
		// wikilink address, and renaming it would break links.
		// A bookmark has no description of its own; the excerpt the archiver
		// fetches lives on the bookmark node, not in the note's frontmatter.
		companionFile, err = companion.Create(ctx, tx, a.Config.Vault.Root, id, layout.BookmarksFolder, b.Title, "", "bookmark")
		return err
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := companion.WriteFile(a.Config.Vault.Root, companionFile); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	startBookmarkArchive(a.Graph, a.Config.Vault.Root, id)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func listBookmarksHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	// Absent keeps today's default of archived and unarchived alike (archiving
	// is success, not removal); "true"/"false" narrow to one state.
	archived := r.URL.Query().Get("archived")
	filter := nodes.BookmarkFilter{IncludeArchived: true}
	switch archived {
	case "":
	case "true":
		filter.ArchivedOnly = true
	case "false":
		filter.IncludeArchived = false
	default:
		http.Error(w, fmt.Sprintf("invalid archived value %q: must be true or false", archived), http.StatusBadRequest)
		return
	}
	// tags is match-any: a bookmark matching at least one listed tag is
	// included (BookmarkFilter.Tags, documented at its declaration).
	if tags := r.URL.Query().Get("tags"); tags != "" {
		filter.Tags = strings.Split(tags, ",")
	}

	ctx := r.Context()
	var list []*nodes.Bookmark
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		list, err = nodes.ListBookmarks(ctx, tx, filter)
		return err
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newBookmarkResponses(list))
}

func getBookmarkHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	ctx := r.Context()

	var b *nodes.Bookmark
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	})
	if errors.Is(err, graph.ErrNotFound) {
		http.Error(w, "bookmark not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newBookmarkResponse(b))
}

// patchBookmarkHandler updates title, description, tags and archive state.
// Every field is optional; an omitted one is left exactly as it was read,
// which is why the update reads the current bookmark first rather than
// building one from the request body alone.
func patchBookmarkHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")

	var req bookmarkPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var b *nodes.Bookmark
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	})
	if errors.Is(err, graph.ErrNotFound) {
		http.Error(w, "bookmark not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Title != nil {
		b.Title = *req.Title
	}
	if req.Description != nil {
		b.Description = *req.Description
	}
	if req.Tags != nil {
		b.Tags = *req.Tags
	}
	if req.Archived != nil {
		switch {
		case *req.Archived && b.ArchivedAt == nil:
			now := time.Now()
			b.ArchivedAt = &now
		case !*req.Archived:
			b.ArchivedAt = nil
		}
	}

	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateBookmark(ctx, tx, *b, nodes.Author{Name: "api"})
	})
	if errors.Is(err, graph.ErrNotFound) {
		http.Error(w, "bookmark not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newBookmarkResponse(b))
}

func addHighlightHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req struct {
		Text string `json:"text"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		http.Error(w, "highlight text is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	highlight := nodes.Highlight{Text: req.Text, Note: req.Note, CreatedAt: time.Now()}

	var b *nodes.Bookmark
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	b.Highlights = append(b.Highlights, highlight)
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateBookmark(ctx, tx, *b, nodes.Author{Name: "api"})
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newHighlightResponse(highlight))
}
