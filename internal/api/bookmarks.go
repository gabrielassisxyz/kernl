package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

func RegisterBookmarkRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("POST /api/bookmarks", func(w http.ResponseWriter, r *http.Request) {
		createBookmarkHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/bookmarks", func(w http.ResponseWriter, r *http.Request) {
		listBookmarksHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/bookmarks/{id}/highlights", func(w http.ResponseWriter, r *http.Request) {
		addHighlightHandler(w, r, a)
	})
}

func createBookmarkHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var id string
	var companion CompanionFile

	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "api"}
		// The URL stands in as the title until the archiver extracts the real
		// one. It is a poor title but a true one, which a placeholder word is
		// not: "Pending" outlived every bookmark that ever carried it, because
		// nothing downstream could tell it apart from a title someone meant.
		b := nodes.Bookmark{URL: req.URL, Title: req.URL}

		var err error
		id, err = nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return err
		}
		// Archiving is asynchronous, so no title exists yet to name the
		// companion note by; it takes the URL, the only identifier available at
		// creation time. It keeps that name afterwards on purpose - the note's
		// file stem is its wikilink address, and renaming it would break links.
		// A bookmark has no description of its own; the excerpt the archiver
		// fetches lives on the bookmark node, not in the note's frontmatter.
		companion, err = CreateCompanionNote(ctx, tx, a, id, layout.BookmarksFolder, req.URL, "", "bookmark")
		return err
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := WriteCompanionFile(a, companion); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Archive (raw HTML + excerpt) in the background so the response is fast,
	// matching the CLI/inbox paths which also archive.
	archiver := bookmarks.NewArchiver(nil, bookmarks.ArchiveDir(a.Config.Vault.Root))
	go func() {
		if err := bookmarks.ArchiveAndPersist(context.Background(), a.Graph, archiver, id); err != nil {
			slog.Warn("bookmark archive failed", "id", id, "error", err)
		}
	}()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func listBookmarksHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	var list []*nodes.Bookmark

	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		// Include archived bookmarks - archiving is success, not removal; the
		// reader should show them (default filter would hide archived ones).
		filter := nodes.BookmarkFilter{IncludeArchived: true}
		if tags := r.URL.Query().Get("tags"); tags != "" {
			filter.Tags = strings.Split(tags, ",")
		}

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
