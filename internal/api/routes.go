package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
	"github.com/gabrielassisxyz/kernl/internal/logging"
	"github.com/gabrielassisxyz/kernl/web"
)

func NewRouter(a *app.App) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", healthHandler(a))

	RegisterBeadRoutes(mux, a)
	RegisterApprovalRoutes(mux)
	RegisterAuditRoutes(mux, a)
	RegisterStreamRoutes(mux, a)
	RegisterEpicRoutes(mux, a)
	RegisterAppRoutes(mux)
	RegisterDARoutes(mux, a)
	RegisterChatRoutes(mux, a)
	RegisterChatResolveRoutes(mux, a)
	RegisterVaultRoutes(mux, a)
	RegisterMemoryRoutes(mux, a)
	RegisterBookmarkRoutes(mux, a)
	RegisterProjectRoutes(mux, a)
	RegisterTaskRoutes(mux, a)
	RegisterIngestRoutes(mux, a)
	RegisterInboxRoutes(mux, a)
	RegisterNotesRoutes(mux, a)
	RegisterTagRoutes(mux, a)
	RegisterNodeSearchRoutes(mux, a)
	RegisterNodeRelatedRoutes(mux, a)
	RegisterEdgeRoutes(mux, a)
	RegisterPlanRoutes(mux, a)

	mux.HandleFunc("POST /api/epics/{id}/run", dispatch.HandleEpicRunAPI(a.Backend, a.Config))
	mux.Handle("GET /", http.FileServerFS(web.FS))

	var h http.Handler = mux
	h = logging.TimingMiddleware(h)
	h = logging.LoggingMiddleware(h)
	h = logging.CorrelationMiddleware(h)
	h = corsMiddleware(h)

	return h
}

// healthHandler reports liveness plus the configured vault location, so the
// shell footer can show where the active vault lives instead of a placeholder.
func healthHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		vaultRoot := ""
		if a.Config != nil {
			vaultRoot = a.Config.Vault.Root
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":     "ok",
			"vaultRoot":  vaultRoot,
			"vaultLabel": vaultDisplayPath(vaultRoot),
		})
	}
}

// vaultDisplayPath abbreviates the user's home directory to "~" for display.
// Returns the unmodified path when it is outside home or home is unknown.
func vaultDisplayPath(root string) string {
	if root == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return root
	}
	rel, err := filepath.Rel(home, root)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return root
	}
	if rel == "." {
		return "~"
	}
	return "~/" + filepath.ToSlash(rel)
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
