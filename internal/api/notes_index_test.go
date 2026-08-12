package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

var testAuthor = nodes.Author{Name: "test"}

// seedVaultNote creates a note node, indexes it at path, and tags it. When
// describes is non-empty the note becomes that entity's companion, which is how
// a note acquires a category other than "note".
func seedVaultNote(t *testing.T, a *app.App, title, path, fmAuthor, describes string, noteTags ...string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{
			Title:  title,
			Body:   "body of " + title,
			Author: fmAuthor,
			Tags:   noteTags,
		}, testAuthor)
		if err != nil {
			return err
		}
		if describes != "" {
			if _, err := edges.Create(ctx, tx, edges.Edge{
				Src: id, Dst: describes, Label: companion.EdgeLabel,
			}, testAuthor); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed note %q: %v", title, err)
	}
	if err := reconcile.Upsert(ctx, a.Graph, id, path, "hash-"+id); err != nil {
		t.Fatalf("index note %q: %v", title, err)
	}
	return id
}

func getNoteIndex(t *testing.T, a *app.App) noteIndexResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/notes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/notes = %d, body %s", rec.Code, rec.Body.String())
	}
	var out noteIndexResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode index: %v", err)
	}
	return out
}

func entryByPath(t *testing.T, index noteIndexResponse, path string) noteIndexEntry {
	t.Helper()
	for _, e := range index.Notes {
		if e.Path == path {
			return e
		}
	}
	t.Fatalf("no index entry for %q; got %d entries", path, len(index.Notes))
	return noteIndexEntry{}
}

// TestNoteIndexCategoryComesFromTheDescribedEntity is the point of the whole
// endpoint. Every file in the vault is a "note" node - a project's companion is
// a separate note joined to it by a describes edge - so reading nodes.type
// would label every row "note", which is exactly the defect the old list had.
func TestNoteIndexCategoryComesFromTheDescribedEntity(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	ctx := context.Background()

	var projectID string
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		projectID, err = nodes.CreateProject(ctx, tx, nodes.Project{Title: "kernl"}, testAuthor)
		return err
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	seedVaultNote(t, a, "kernl", "projects/kernl.md", "", projectID)
	seedVaultNote(t, a, "a plain thought", "notes/thought.md", "", "")

	index := getNoteIndex(t, a)
	if got := entryByPath(t, index, "projects/kernl.md").Category; got != "project" {
		t.Errorf("companion note category = %q, want %q", got, "project")
	}
	if got := entryByPath(t, index, "notes/thought.md").Category; got != "note" {
		t.Errorf("plain note category = %q, want %q", got, "note")
	}
}

// A note describing an entity that was deleted falls back to "note" rather than
// vanishing from the index or reporting an empty category.
func TestNoteIndexCategoryFallsBackWhenTheEntityIsGone(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	ctx := context.Background()

	var projectID string
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		projectID, err = nodes.CreateProject(ctx, tx, nodes.Project{Title: "retired"}, testAuthor)
		return err
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	seedVaultNote(t, a, "retired", "projects/retired.md", "", projectID)

	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.DeleteProject(ctx, tx, projectID, testAuthor)
	}); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	index := getNoteIndex(t, a)
	if got := entryByPath(t, index, "projects/retired.md").Category; got != "note" {
		t.Errorf("orphaned companion category = %q, want %q", got, "note")
	}
}

// The badge that says "this one is not yours" hangs off this mapping, so the
// endpoint has to resolve authorship the same way the reconciler wrote it -
// notably "da" becoming "agent:da" and an absent value becoming "human".
func TestNoteIndexResolvesAuthorship(t *testing.T) {
	a, _ := newCompanionTestApp(t)

	seedVaultNote(t, a, "mine", "notes/mine.md", "", "")
	seedVaultNote(t, a, "the DA's", "notes/da.md", "da", "")
	seedVaultNote(t, a, "named human", "notes/named.md", "user", "")

	index := getNoteIndex(t, a)
	for path, want := range map[string]string{
		"notes/mine.md":  "human",
		"notes/da.md":    "agent:da",
		"notes/named.md": "user",
	} {
		if got := entryByPath(t, index, path).Author; got != want {
			t.Errorf("%s author = %q, want %q", path, got, want)
		}
	}
}

func TestNoteIndexCarriesTagsAndTimestamps(t *testing.T) {
	a, _ := newCompanionTestApp(t)

	seedVaultNote(t, a, "tagged", "notes/tagged.md", "", "", "zulu", "alpha")
	seedVaultNote(t, a, "bare", "notes/bare.md", "", "")

	index := getNoteIndex(t, a)

	tagged := entryByPath(t, index, "notes/tagged.md")
	if len(tagged.Tags) != 2 || tagged.Tags[0] != "alpha" || tagged.Tags[1] != "zulu" {
		t.Errorf("tags = %v, want [alpha zulu] in name order", tagged.Tags)
	}
	if tagged.CreatedAt == "" || tagged.UpdatedAt == "" {
		t.Errorf("timestamps missing: created %q, updated %q", tagged.CreatedAt, tagged.UpdatedAt)
	}

	// An untagged note reports an empty list, never null: the client iterates
	// it without a guard.
	bare := entryByPath(t, index, "notes/bare.md")
	if bare.Tags == nil {
		t.Error("an untagged note should carry [] rather than null")
	}
}

func TestNoteIndexPinRoundTrip(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	id := seedVaultNote(t, a, "worth keeping", "notes/keep.md", "", "")

	if entryByPath(t, getNoteIndex(t, a), "notes/keep.md").Pinned {
		t.Fatal("a fresh note should not be pinned")
	}

	body := bytes.NewBufferString(`{"pinned":true}`)
	rec := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/notes/"+id, body))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH pin = %d, body %s", rec.Code, rec.Body.String())
	}
	if !entryByPath(t, getNoteIndex(t, a), "notes/keep.md").Pinned {
		t.Error("note should be pinned after the PATCH")
	}

	rec = httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/notes/"+id,
		bytes.NewBufferString(`{"pinned":false}`)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH unpin = %d, body %s", rec.Code, rec.Body.String())
	}
	if entryByPath(t, getNoteIndex(t, a), "notes/keep.md").Pinned {
		t.Error("note should be unpinned after the second PATCH")
	}
}

func TestNoteIndexPinRejectsUnknownNoteAndEmptyBody(t *testing.T) {
	a, _ := newCompanionTestApp(t)

	rec := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/notes/no-such-id",
		bytes.NewBufferString(`{"pinned":true}`)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("PATCH on an unknown note = %d, want 404", rec.Code)
	}

	id := seedVaultNote(t, a, "present", "notes/present.md", "", "")
	rec = httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/notes/"+id,
		bytes.NewBufferString(`{}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH with no pinned field = %d, want 400", rec.Code)
	}
}

// The tag name travels in the body precisely so a slashed name survives, so
// that is the name the test pins.
func TestNoteIndexTagPinRoundTrip(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	seedVaultNote(t, a, "audited", "notes/audited.md", "", "", "sys/audit")

	if got := getNoteIndex(t, a).PinnedTags; len(got) != 0 {
		t.Fatalf("pinnedTags on a fresh vault = %v, want empty", got)
	}

	rec := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/notes/tags",
		bytes.NewBufferString(`{"name":"sys/audit","pinned":true}`)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH tag pin = %d, body %s", rec.Code, rec.Body.String())
	}

	got := getNoteIndex(t, a).PinnedTags
	if len(got) != 1 || got[0] != "sys/audit" {
		t.Errorf("pinnedTags = %v, want [sys/audit]", got)
	}
}

func TestNoteIndexTagPinRejectsUnknownTag(t *testing.T) {
	a, _ := newCompanionTestApp(t)

	rec := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/notes/tags",
		bytes.NewBufferString(`{"name":"nothing-carries-this","pinned":true}`)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("PATCH on an unknown tag = %d, want 404", rec.Code)
	}
}

// A node with no vault file has no business in a list of vault files, and a
// tombstoned note has to leave it.
func TestNoteIndexHoldsOnlyLiveVaultFiles(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	ctx := context.Background()

	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateProject(ctx, tx, nodes.Project{Title: "no companion"}, testAuthor)
		return err
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	doomed := seedVaultNote(t, a, "doomed", "notes/doomed.md", "", "")
	seedVaultNote(t, a, "alive", "notes/alive.md", "", "")

	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.DeleteNote(ctx, tx, doomed, testAuthor)
	}); err != nil {
		t.Fatalf("delete note: %v", err)
	}

	index := getNoteIndex(t, a)
	if len(index.Notes) != 1 {
		t.Fatalf("index holds %d entries, want 1", len(index.Notes))
	}
	if index.Notes[0].Path != "notes/alive.md" {
		t.Errorf("index kept %q, want notes/alive.md", index.Notes[0].Path)
	}
}

func TestNoteIndexIsEmptyNotNull(t *testing.T) {
	a, _ := newCompanionTestApp(t)

	rec := httptest.NewRecorder()
	NewRouter(a).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/notes", nil))
	if body := rec.Body.String(); !bytes.Contains([]byte(body), []byte(`"notes":[]`)) ||
		!bytes.Contains([]byte(body), []byte(`"pinnedTags":[]`)) {
		t.Errorf("an empty vault should answer with empty arrays, got %s", body)
	}
}
