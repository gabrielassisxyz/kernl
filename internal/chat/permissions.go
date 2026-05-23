package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault"
)

// GraphPermissionChecker evaluates permissions using the graph Vault.
type GraphPermissionChecker struct {
	App *app.App
}

// NewGraphPermissionChecker creates a permission checker backed by the graph.
func NewGraphPermissionChecker(app *app.App) *GraphPermissionChecker {
	return &GraphPermissionChecker{App: app}
}

// CanRead determines whether the agent may read a node.
// Returns (allowed, denialReason, error). DenialReason is empty when allowed.
func (g *GraphPermissionChecker) CanRead(ctx context.Context, nodeID string) (bool, DenialReason, error) {
	var note *nodes.Note
	var attrsRaw string
	err := g.App.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		note, err = nodes.GetNote(ctx, tx, nodeID)
		if err != nil {
			return err
		}
		row := tx.QueryRow(`SELECT attrs FROM nodes WHERE id = ? AND deleted_at IS NULL`, nodeID)
		return row.Scan(&attrsRaw)
	})
	if err != nil {
		if errors.Is(err, graph.ErrNotFound) {
			return false, "node not found", nil
		}
		return false, "", err
	}

	// Local check: tags containing "confidencial".
	for _, tag := range note.Tags {
		if tag == "confidencial" {
			return false, "node marked private", nil
		}
	}

	// Local check: attrs.visibility == "private".
	var attrs struct {
		Visibility string `json:"visibility"`
		Path       string `json:"path"`
	}
	if attrsRaw != "" {
		_ = json.Unmarshal([]byte(attrsRaw), &attrs)
	}
	if attrs.Visibility == "private" {
		return false, "node marked private", nil
	}

	// Global check: evaluate .kernl-policies if node has a path.
	if attrs.Path != "" && g.App.Config != nil && g.App.Config.Vault.Root != "" {
		parser := vault.NewPolicyParser(g.App.Config.Vault.Root)
		relPath := strings.TrimPrefix(attrs.Path, "/")
		if !parser.CanReadGlobal(relPath) {
			return false, "forbidden by global policy", nil
		}
	}

	return true, "", nil
}
