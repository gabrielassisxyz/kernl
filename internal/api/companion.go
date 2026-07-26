package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
	"github.com/google/uuid"
)

// companionEdgeLabel links a companion note to the entity it describes.
const companionEdgeLabel = "describes"

var companionSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// companionSlug builds a filesystem-safe slug from a label, falling back to the
// node id when the label has no usable characters (e.g. a bookmark titled by URL).
func companionSlug(label, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	s = companionSlugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return fallback
	}
	return s
}

// companionSuffixAttempts bounds the `-2`, `-3`, ... search before falling back
// to the note id. The suffixed name is for the human reading the vault, so a long
// walk defeats its own purpose; past this many collisions the unique-by-
// construction name is the better answer.
const companionSuffixAttempts = 50

// freeCompanionPath picks a vault-relative path for the companion note that no
// other note claims. Two entities with the same title slugify identically, and
// note_paths.path is UNIQUE - so without this the second INSERT fails and, since
// the entity and its companion share one transaction, the whole entity creation
// is rolled back. A duplicate title is ordinary user behaviour, not an error.
//
// Disk is checked alongside the table because a companion folder may also hold
// notes written by hand: those have no note_paths row yet (the reconciler adopts
// them later), and overwriting one would destroy content the user typed.
func freeCompanionPath(a *app.App, tx *graph.WriteTx, folder, slug, noteID string) (string, error) {
	for attempt := 1; attempt <= companionSuffixAttempts; attempt++ {
		name := slug
		if attempt > 1 {
			name = fmt.Sprintf("%s-%d", slug, attempt)
		}
		rel := filepath.ToSlash(filepath.Join(folder, name+".md"))
		taken, err := companionPathTaken(a, tx, rel)
		if err != nil {
			return "", err
		}
		if !taken {
			return rel, nil
		}
	}
	// uuid v7: unique by construction, so this needs no further probing.
	return filepath.ToSlash(filepath.Join(folder, slug+"-"+noteID+".md")), nil
}

// companionPathTaken reports whether relPath is already claimed by a note_paths
// row or by a file in the vault.
func companionPathTaken(a *app.App, tx *graph.WriteTx, relPath string) (bool, error) {
	var claimed int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM note_paths WHERE path = ?`, relPath).Scan(&claimed); err != nil {
		return false, fmt.Errorf("companion: probe note_paths: %w", err)
	}
	if claimed > 0 {
		return true, nil
	}
	root := a.Config.Vault.Root
	if root == "" {
		return false, nil
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(relPath)))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("companion: probe vault file: %w", err)
}

// createCompanionNote creates a real markdown note that describes an entity
// (project/task/bookmark) so the user can annotate it and the wikilink resolver
// can index links from it.
//
// The note is wired three ways, consistent with how reconcile.go represents
// notes so the reconciler later adopts the file instead of duplicating it:
//   - a `note` node (created with an explicit id),
//   - a markdown file under <folder>/<slug>.md whose frontmatter id == the node id,
//   - a note_paths(uuid, path, content_hash) row whose hash matches the file bytes
//     (so ColdStart classifies the file as samePath && sameHash → no-op).
//
// The entity creation and the note node/edge/note_paths rows are committed in
// the SAME write transaction (passed in by the caller). The markdown file is
// written to disk afterwards, by the caller, via writeCompanionFile.
//
// TODO(6A): lifecycle (rename/delete) sync out of scope for now - renaming or
// deleting the entity does not yet rename/remove its companion note.
func CreateCompanionNote(ctx context.Context, tx *graph.WriteTx, a *app.App, entityID, folder, label string, tags ...string) (CompanionFile, error) {
	noteID := uuid.Must(uuid.NewV7()).String()
	slug := companionSlug(label, noteID)
	relPath, err := freeCompanionPath(a, tx, folder, slug, noteID)
	if err != nil {
		return CompanionFile{}, err
	}

	title := strings.TrimSpace(label)
	if title == "" {
		title = slug
	}
	body := fmt.Sprintf("Notes for [[%s|%s]].\n", entityID, title)

	// Tags belong in YAML frontmatter (and on the note node), not as literal
	// "#tag" text appended to the body - the body form never reached the tag
	// index and read as noise in the note. Leading '#' is tolerated for callers.
	cleanTags := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimPrefix(strings.TrimSpace(t), "#")
		if t != "" {
			cleanTags = append(cleanTags, t)
		}
	}

	fileBytes := renderCompanionMarkdown(noteID, title, body, cleanTags)
	contentHash := reconcile.HashBytes(fileBytes)

	if _, err := nodes.CreateNote(ctx, tx, nodes.Note{
		ID:    noteID,
		Title: title,
		Body:  body,
		Tags:  cleanTags,
	}, nodes.Author{Name: "api"}); err != nil {
		return CompanionFile{}, fmt.Errorf("companion: create note: %w", err)
	}

	// describes: companion note -> entity.
	if _, err := edges.Create(ctx, tx, edges.Edge{
		Src:   noteID,
		Dst:   entityID,
		Label: companionEdgeLabel,
	}, nodes.Author{Name: "api"}); err != nil {
		return CompanionFile{}, fmt.Errorf("companion: create describes edge: %w", err)
	}

	// note_paths mapping with the on-disk hash so the reconciler adopts the file.
	if _, err := tx.Exec(
		`INSERT INTO note_paths (uuid, path, content_hash, updated_at)
		 VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
		noteID, relPath, contentHash,
	); err != nil {
		return CompanionFile{}, fmt.Errorf("companion: insert note_paths: %w", err)
	}

	return CompanionFile{relPath: relPath, bytes: fileBytes}, nil
}

// companionFile carries the bytes to write to disk after the transaction commits.
type CompanionFile struct {
	relPath string
	bytes   []byte
}

// renderCompanionMarkdown builds the markdown file content with a frontmatter
// id equal to the note node id, so reconcile.OnCreate/ColdStart match the
// existing node by id rather than creating a duplicate.
//
// The block is MARSHALLED, not concatenated. Pasting a title in raw wrote
// `title: AI-SEO: llms.txt` - a second colon YAML reads as a nested mapping  -
// and the file became unparseable the moment a title contained ":", "#" or a
// leading "-". The marshaller quotes what needs quoting; a string builder cannot
// know what that is.
func renderCompanionMarkdown(id, title, body string, tags []string) []byte {
	// A local shape rather than frontmatter.Frontmatter: that struct is the READ
	// contract and has no omitempty, so marshalling it would stamp empty author
	// and origin lines onto every file.
	fm, err := yaml.Marshal(struct {
		ID    string   `yaml:"id"`
		Title string   `yaml:"title"`
		Tags  []string `yaml:"tags,omitempty"`
	}{ID: id, Title: title, Tags: tags})
	if err != nil {
		// Marshalling plain strings cannot fail; if it somehow does, a file with
		// no frontmatter is still better than a corrupt one (the reconciler
		// injects an id on cold start).
		slog.Error("render companion frontmatter", "id", id, "error", err)
		return []byte(body)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n")
	b.WriteString(body)
	return []byte(b.String())
}

// writeCompanionFile writes the companion markdown to the vault. It is a no-op
// when no vault root is configured (the node + edge are still created), so the
// in-memory test harness and headless contexts do not error.
func WriteCompanionFile(a *app.App, cf CompanionFile) error {
	root := a.Config.Vault.Root
	if root == "" {
		return nil
	}
	full := filepath.Join(root, filepath.FromSlash(cf.relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("companion: mkdir: %w", err)
	}
	if err := os.WriteFile(full, cf.bytes, 0o644); err != nil {
		return fmt.Errorf("companion: write file: %w", err)
	}
	return nil
}
