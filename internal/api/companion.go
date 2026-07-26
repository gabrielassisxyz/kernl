package api

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/gabrielassisxyz/kernl/internal/notes"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
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
// description is the entity's own short description, mirrored into the note's
// frontmatter. Pass "" for an entity whose description is not kept in sync:
// stamping a value that later drifts is worse than not showing one.
//
// TODO(6A): the entity is never renamed on disk. The companion keeps the path
// it was created with even when the entity's title changes - the path is no
// longer a function of the title (freeCompanionPath may suffix it), so deriving
// a new name from a new title would re-couple the two. Deleting an entity does
// remove its companion; renaming deliberately does not.
func CreateCompanionNote(ctx context.Context, tx *graph.WriteTx, a *app.App, entityID, folder, label, description string, tags ...string) (CompanionFile, error) {
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

	fileBytes := renderCompanionMarkdown(noteID, title, description, body, cleanTags)
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
// The zero value means "nothing to write" and WriteCompanionFile skips it, so a
// caller that only sometimes touches the companion needs no extra flag.
type CompanionFile struct {
	relPath string
	bytes   []byte
}

// SyncCompanionDescription rewrites the description in the frontmatter of the
// companion note describing entityID, and returns the bytes for the caller to
// write after the transaction commits.
//
// The note is found through its describes edge, never by re-slugifying the
// entity's title: freeCompanionPath may have suffixed the file name (-2, -3, or
// the note id), so the title has not determined the path since duplicate titles
// became legal. Re-deriving it would rewrite a different entity's note.
//
// The new content hash is written to note_paths INSIDE the caller's transaction.
// A file kernl rewrote without updating the hash reads as user drift on the next
// cold start: the reconciler takes the OnChange branch and records a revision for
// an edit nobody made, and the same divergence in its harder form (a live note
// with no cache row) is what wedged one real vault for a month.
//
// Returns the zero CompanionFile - no error - whenever the file must not be
// touched: no vault configured, no companion, the file is gone, its frontmatter
// is unreadable, it belongs to another note, or the description already matches.
func SyncCompanionDescription(ctx context.Context, tx *graph.WriteTx, a *app.App, entityID, description string) (CompanionFile, error) {
	root := a.Config.Vault.Root
	if root == "" {
		return CompanionFile{}, nil
	}
	note, found, err := companionOf(tx, entityID)
	if err != nil || !found {
		return CompanionFile{}, err
	}

	full := filepath.Join(root, filepath.FromSlash(note.relPath))
	raw, err := os.ReadFile(full)
	if err != nil {
		// A companion whose file the user deleted stays deleted: rewriting it here
		// would resurrect a note they threw away.
		slog.Warn("companion: description not synced, file unreadable", "note_id", note.id, "path", note.relPath, "error", err)
		return CompanionFile{}, nil
	}

	updated, ok := rewriteCompanionDescription(raw, note, description)
	if !ok {
		slog.Warn("companion: description not synced, file is not one kernl can rewrite", "note_id", note.id, "path", note.relPath)
		return CompanionFile{}, nil
	}
	if bytes.Equal(updated, raw) {
		return CompanionFile{}, nil
	}

	if _, err := tx.Exec(
		`UPDATE note_paths SET content_hash = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE uuid = ?`,
		reconcile.HashBytes(updated), note.id,
	); err != nil {
		return CompanionFile{}, fmt.Errorf("companion: refresh note_paths hash: %w", err)
	}
	return CompanionFile{relPath: note.relPath, bytes: updated}, nil
}

// companionRef is the companion note of an entity, as the graph knows it.
type companionRef struct {
	id      string
	title   string
	relPath string
}

// companionOf resolves the note describing entityID through the describes edge
// and the path cache. found is false when the entity has no companion (created
// before companions existed, or its note was deleted) - not an error.
func companionOf(tx *graph.WriteTx, entityID string) (companionRef, bool, error) {
	var ref companionRef
	err := tx.QueryRow(
		`SELECT n.id, n.title FROM edges e
		 JOIN nodes n ON n.id = e.src AND n.type = 'note' AND n.deleted_at IS NULL
		 WHERE e.dst = ? AND e.label = ?`,
		entityID, companionEdgeLabel,
	).Scan(&ref.id, &ref.title)
	if err == sql.ErrNoRows {
		return companionRef{}, false, nil
	}
	if err != nil {
		return companionRef{}, false, fmt.Errorf("companion: lookup describes edge: %w", err)
	}
	err = tx.QueryRow(`SELECT path FROM note_paths WHERE uuid = ?`, ref.id).Scan(&ref.relPath)
	if err == sql.ErrNoRows {
		return companionRef{}, false, nil
	}
	if err != nil {
		return companionRef{}, false, fmt.Errorf("companion: lookup note path: %w", err)
	}
	return ref, true, nil
}

// rewriteCompanionDescription returns the file with its frontmatter re-rendered
// around the new description, byte-for-byte preserving everything after the
// closing fence. The body is the user's working material and is never the thing
// being edited here.
//
// The frontmatter block as a whole is regenerated rather than patched in place,
// which is what "the file loses, the node wins" means in practice: keys kernl
// does not write are dropped. id, title and tags are carried over FROM THE FILE
// rather than from the graph, because the file is the source of truth for those
// everywhere else in the reconciler - a tag added by hand survives a description
// edit even if the server never saw the file change.
//
// ok is false when the file is not one kernl should rewrite: no frontmatter,
// unparseable YAML, or an id naming a different note.
func rewriteCompanionDescription(raw []byte, note companionRef, description string) ([]byte, bool) {
	block, body := notes.SplitFrontmatter(string(raw))
	if block == "" {
		return nil, false
	}
	fm, err := frontmatter.Parse(raw)
	if err != nil {
		return nil, false
	}
	if fm.ID != "" && fm.ID != note.id {
		return nil, false
	}
	title := fm.Title
	if title == "" {
		title = note.title
	}
	return renderCompanionMarkdown(note.id, title, description, body, fm.Tags), true
}

// renderCompanionMarkdown builds the markdown file content with a frontmatter
// id equal to the note node id, so reconcile.OnCreate/ColdStart match the
// existing node by id rather than creating a duplicate.
//
// description sits in the frontmatter and not in the body on purpose. The
// frontmatter is already machine-managed, so the "this is mine / this is the
// system's" boundary exists without inventing one, and an editor renders it as a
// properties panel - visible at the top of the note without competing with the
// prose underneath. It is written empty-key-omitted, like tags.
//
// The block is MARSHALLED, not concatenated. Pasting a title in raw wrote
// `title: AI-SEO: llms.txt` - a second colon YAML reads as a nested mapping  -
// and the file became unparseable the moment a title contained ":", "#" or a
// leading "-". The marshaller quotes what needs quoting; a string builder cannot
// know what that is.
func renderCompanionMarkdown(id, title, description, body string, tags []string) []byte {
	// A local shape rather than frontmatter.Frontmatter: that struct is the READ
	// contract and has no omitempty, so marshalling it would stamp empty author
	// and origin lines onto every file.
	fm, err := yaml.Marshal(struct {
		ID          string   `yaml:"id"`
		Title       string   `yaml:"title"`
		Description string   `yaml:"description,omitempty"`
		Tags        []string `yaml:"tags,omitempty"`
	}{ID: id, Title: title, Description: description, Tags: tags})
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
	if root == "" || cf.relPath == "" {
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
