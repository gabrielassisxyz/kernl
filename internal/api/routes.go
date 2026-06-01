package api

import (
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/logging"
	"github.com/gabrielassisxyz/kernl/web"
)

func NewRouter(a *app.App) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", healthHandler)

	RegisterBeadRoutes(mux, a)
	RegisterApprovalRoutes(mux)
	RegisterStreamRoutes(mux, a)
	RegisterEpicRoutes(mux, a)
	RegisterAppRoutes(mux)
	RegisterDARoutes(mux, a)
	RegisterChatRoutes(mux, a)
	RegisterChatResolveRoutes(mux, a)
	RegisterVaultRoutes(mux, a)
	RegisterMemoryRoutes(mux, a)
	RegisterBookmarkRoutes(mux, a)
	RegisterIngestRoutes(mux, a)
	mux.Handle("GET /", http.FileServerFS(web.FS))

	var h http.Handler = mux
	h = logging.TimingMiddleware(h)
	h = logging.LoggingMiddleware(h)
	h = logging.CorrelationMiddleware(h)
	h = corsMiddleware(h)

	return h
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
