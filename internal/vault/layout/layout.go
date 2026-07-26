// Package layout names the vault folders kernl writes into on its own, and
// reports notes sitting in them that kernl did not put there.
//
// The folders are a tidiness convention and nothing more: no code may infer
// "this note describes a task" from its path. Identity lives in the graph, in
// the edge that anchors the note to the entity it was created for. This package
// exists so the folder names have one definition instead of a literal repeated
// at every call site, and so a note that drifted from the convention can be
// surfaced rather than policed.
package layout

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
)

// Vault-relative folders kernl materializes entities into.
const (
	TasksFolder     = "tasks"
	ProjectsFolder  = "projects"
	BookmarksFolder = "bookmarks"
	DefaultDAFolder = "DA"
)

// Anchor edge labels: the edge that ties a generated note to the node it was
// written for. A companion note describes its entity. A DA primer is
// prepared_for the capture it briefs, but that edge does not survive the
// capture being processed away - what remains is the briefing edge the
// resulting task or note points back with, so either one anchors it.
const (
	DescribesLabel   = "describes"
	PreparedForLabel = "prepared_for"
	BriefingLabel    = "briefing"
)

// Generated pairs a folder with the edges that make a note in it legitimate.
// Direction matters: an anchor is either an edge the note carries (outgoing) or
// one pointed at it (incoming).
type Generated struct {
	Folder   string
	Outgoing []string
	Incoming []string
}

// AnchorLabels lists every label that anchors a note in this folder, in the
// order they are checked - used to explain a miss.
func (g Generated) AnchorLabels() []string {
	return append(append([]string{}, g.Outgoing...), g.Incoming...)
}

// GeneratedFolders lists every folder kernl writes notes into. daFolder is
// configurable (inbox.da_subdir), so it is passed in rather than assumed; an
// empty value falls back to the default.
func GeneratedFolders(daFolder string) []Generated {
	if strings.TrimSpace(daFolder) == "" {
		daFolder = DefaultDAFolder
	}
	return []Generated{
		{Folder: TasksFolder, Outgoing: []string{DescribesLabel}},
		{Folder: ProjectsFolder, Outgoing: []string{DescribesLabel}},
		{Folder: BookmarksFolder, Outgoing: []string{DescribesLabel}},
		{Folder: daFolder, Outgoing: []string{PreparedForLabel}, Incoming: []string{BriefingLabel}},
	}
}

// Orphan is a note found in a generated folder that no entity claims.
type Orphan struct {
	Path   string // vault-relative
	Reason string
}

// ScanOrphans walks the generated folders and returns the notes in them that
// are not anchored to an entity.
//
// This detects drift, it does not forbid it: writing a note by hand inside
// tasks/ is legitimate (the vault is the user's), it just means the folder no
// longer says what the graph says. Reporting the divergence lets the user
// decide file by file; silently adopting or refusing the file would decide for
// them.
func ScanOrphans(ctx context.Context, g *graph.Graph, vaultRoot, daFolder string) ([]Orphan, error) {
	if strings.TrimSpace(vaultRoot) == "" {
		return nil, nil
	}
	var orphans []Orphan
	for _, gen := range GeneratedFolders(daFolder) {
		dir := filepath.Join(vaultRoot, filepath.FromSlash(gen.Folder))
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("layout: read %s: %w", gen.Folder, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
				continue
			}
			relPath := gen.Folder + "/" + e.Name()
			reason, err := orphanReason(ctx, g, filepath.Join(dir, e.Name()), relPath, gen)
			if err != nil {
				return nil, err
			}
			if reason != "" {
				orphans = append(orphans, Orphan{Path: relPath, Reason: reason})
			}
		}
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Path < orphans[j].Path })
	return orphans, nil
}

// orphanReason returns why the file is an orphan, or "" when it is anchored.
func orphanReason(ctx context.Context, g *graph.Graph, absPath, relPath string, gen Generated) (string, error) {
	noteID, err := noteIDFor(ctx, g, absPath, relPath)
	if err != nil {
		return "", err
	}
	if noteID == "" {
		return "not indexed as a note", nil
	}

	var anchored bool
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		out, err := edges.Outgoing(ctx, tx, noteID)
		if err != nil {
			return err
		}
		if hasAnyLabel(out, gen.Outgoing) {
			anchored = true
			return nil
		}
		if len(gen.Incoming) == 0 {
			return nil
		}
		in, err := edges.Incoming(ctx, tx, noteID)
		if err != nil {
			return err
		}
		anchored = hasAnyLabel(in, gen.Incoming)
		return nil
	}); err != nil {
		return "", fmt.Errorf("layout: edges of %s: %w", relPath, err)
	}
	if anchored {
		return "", nil
	}
	return "no " + strings.Join(gen.AnchorLabels(), " or ") + " edge", nil
}

func hasAnyLabel(list []edges.Edge, labels []string) bool {
	for _, e := range list {
		for _, want := range labels {
			if e.Label == want {
				return true
			}
		}
	}
	return false
}

// noteIDFor resolves the note behind a file: the path cache first, then the
// frontmatter id for a file the reconciler has not adopted yet.
func noteIDFor(ctx context.Context, g *graph.Graph, absPath, relPath string) (string, error) {
	var noteID string
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		row := tx.QueryRow(`SELECT uuid FROM note_paths WHERE path = ?`, relPath)
		if err := row.Scan(&noteID); err != nil {
			noteID = "" // sql.ErrNoRows and anything else fall back to the file
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("layout: lookup %s: %w", relPath, err)
	}
	if noteID != "" {
		return noteID, nil
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", nil // unreadable file: reported as unindexed, not fatal
	}
	fm, err := frontmatter.Parse(raw)
	if err != nil {
		return "", nil
	}
	return fm.ID, nil
}
