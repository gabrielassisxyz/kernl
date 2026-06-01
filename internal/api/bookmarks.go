package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
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

	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "api"}
		b := nodes.Bookmark{URL: req.URL, Title: "Pending"}

		var err error
		id, err = nodes.CreateBookmark(ctx, tx, b, author)
		return err
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func listBookmarksHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	var list []*nodes.Bookmark

	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		filter := nodes.BookmarkFilter{}
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

	json.NewEncoder(w).Encode(list)
}

func addHighlightHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
