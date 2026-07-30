package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// fakeBr is a br binary that records the argv it was called with and replies
// from a scripted table. It exists because the shapes this adapter has to get
// right - which envelope each command returns, which end of a dependency row
// is the child - are properties of br's wire format, and a mock at the Go level
// would assert the adapter against the author's own reading of it.
type fakeBr struct {
	t   *testing.T
	dir string
}

// newFakeBr writes a shell script named br into a temp dir, puts that dir on
// PATH, and points BR_BIN at it. replies maps a subcommand prefix (the argv
// after the global flags, joined by spaces) to the stdout it should produce.
func newFakeBr(t *testing.T, replies map[string]string) *fakeBr {
	t.Helper()
	dir := t.TempDir()
	f := &fakeBr{t: t, dir: dir}

	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + filepath.Join(dir, "calls.log") + "\n"
	for prefix, out := range replies {
		// A case arm per scripted prefix; first match wins, so more specific
		// prefixes must be registered by longer keys - the table is small
		// enough that the tests keep them unambiguous.
		script += "case \"$*\" in\n  *'" + prefix + "'*) cat <<'BREOF'\n" + out + "\nBREOF\n  exit 0;;\nesac\n"
	}
	script += "exit 0\n"

	path := filepath.Join(dir, "br")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BR_BIN", path)
	return f
}

func (f *fakeBr) calledWith() []string {
	raw, err := os.ReadFile(filepath.Join(f.dir, "calls.log"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

// brRepo is a repository whose .beads/ holds a br database.
func brRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".beads", "beads.db"), []byte("sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The tracker follows the repository. Routing used to be decided once, always
// as bd, so naming a br repository handed its path to a backend that runs bd.
func TestAutoRouteFromConfigFollowsTheRepositorysTracker(t *testing.T) {
	br := brRepo(t)
	bd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bd, ".beads", "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{
		{Path: bd, MemoryManager: "beads"},
		{Path: br, MemoryManager: "br"},
	}}}

	first, err := AutoRouteFromConfig(cfg, bd)
	if err != nil {
		t.Fatalf("AutoRouteFromConfig(bd): %v", err)
	}
	if _, ok := first.(*BdCliBackend); !ok {
		t.Errorf("a beads repo must route to the bd backend, got %T", first)
	}

	// The second registered repository is the one repos[0] used to hide.
	second, err := AutoRouteFromConfig(cfg, br)
	if err != nil {
		t.Fatalf("AutoRouteFromConfig(br): %v", err)
	}
	if _, ok := second.(*BrCliBackend); !ok {
		t.Errorf("a br repo must route to the br backend, got %T", second)
	}
}

func TestBrDatabasePath(t *testing.T) {
	t.Run("finds the one database", func(t *testing.T) {
		repo := brRepo(t)
		got, err := BrDatabasePath(repo)
		if err != nil {
			t.Fatalf("BrDatabasePath: %v", err)
		}
		if got != filepath.Join(repo, ".beads", "beads.db") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("no database fails loud", func(t *testing.T) {
		_, err := BrDatabasePath(t.TempDir())
		if err == nil {
			t.Fatal("expected a refusal")
		}
		if !strings.Contains(err.Error(), "br init") {
			t.Errorf("the error must say how to fix it, got: %v", err)
		}
	})

	t.Run("two databases fail rather than pick", func(t *testing.T) {
		repo := brRepo(t)
		if err := os.WriteFile(filepath.Join(repo, ".beads", "other.db"), []byte("sqlite"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := BrDatabasePath(repo); err == nil {
			t.Fatal("expected a refusal rather than an arbitrary choice")
		}
	})
}

// Every call has to carry --db. br discovers its database by walking up from
// the working directory, and the orchestrator and its agents both work inside
// worktrees that are nowhere near the repository.
func TestBrCliPinsTheDatabaseOnEveryCall(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"show kb-1": `[{"id":"kb-1","title":"a bead","status":"implementation"}]`,
	})

	be := NewBrCliBackend(repo)
	if _, err := be.Get("kb-1", repo); err != nil {
		t.Fatalf("Get: %v", err)
	}

	calls := fake.calledWith()
	if len(calls) == 0 {
		t.Fatal("the adapter must invoke br")
	}
	want := "--db " + filepath.Join(repo, ".beads", "beads.db")
	if !strings.Contains(calls[0], want) {
		t.Errorf("call %q must pin the database with %q", calls[0], want)
	}
}

func TestBrCliGet(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"show kb-1": `[{"id":"kb-1","title":"a bead","description":"why","status":"implementation","priority":2,"issue_type":"task","labels":["wf:profile:worker"],"parent":"ep-1","dependencies":[{"id":"kb-0","dependency_type":"blocks"}]}]`,
	})

	bead, err := NewBrCliBackend(repo).Get("kb-1", repo)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if bead.ID != "kb-1" || bead.Title != "a bead" || bead.State != "implementation" {
		t.Errorf("got %+v", bead)
	}
	if bead.ParentID != "ep-1" {
		t.Errorf("ParentID = %q, want ep-1", bead.ParentID)
	}
	if len(bead.Dependencies) != 1 {
		t.Fatalf("expected one dependency, got %d", len(bead.Dependencies))
	}
	// The issue being shown is the dependent; the entry names what it depends
	// on. LoadEpic reads exactly this to orient the epic's DAG.
	if got := bead.Dependencies[0]; got.SourceID != "kb-1" || got.TargetID != "kb-0" {
		t.Errorf("dependency = %+v, want source kb-1 -> target kb-0", got)
	}
}

// br has no --parent. Children come from the reverse dependency lookup and are
// then fetched, because `br list` never returns the dependencies the epic's DAG
// is built from.
func TestBrCliListByParent(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"dep list ep-1": `[{"issue_id":"kb-1","depends_on_id":"ep-1","type":"parent-child"},{"issue_id":"kb-2","depends_on_id":"ep-1","type":"parent-child"}]`,
		"show kb-1 kb-2": `[{"id":"kb-1","title":"first","status":"ready_for_implementation","dependencies":[{"id":"ep-1","dependency_type":"parent-child"}]},
{"id":"kb-2","title":"second","status":"ready_for_implementation","dependencies":[{"id":"kb-1","dependency_type":"blocks"},{"id":"ep-1","dependency_type":"parent-child"}]}]`,
	})

	beads, err := NewBrCliBackend(repo).List(&BeadListFilters{Parent: "ep-1"}, repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(beads) != 2 {
		t.Fatalf("expected 2 children, got %d", len(beads))
	}
	// Getting the direction wrong returns the epic's own parents - nothing -
	// and the run reports success having executed no work.
	joined := strings.Join(fake.calledWith(), "\n")
	if !strings.Contains(joined, "--direction up") {
		t.Error("children must come from the reverse lookup (--direction up)")
	}
	// The children must arrive with their edges, or the DAG is a flat list and
	// every bead runs in parallel regardless of what it waits on.
	var second Bead
	for _, b := range beads {
		if b.ID == "kb-2" {
			second = b
		}
	}
	foundBlocker := false
	for _, d := range second.Dependencies {
		if d.TargetID == "kb-1" && d.Type == "blocks" {
			foundBlocker = true
		}
	}
	if !foundBlocker {
		t.Errorf("kb-2 must carry its blocks edge on kb-1, got %+v", second.Dependencies)
	}
}

func TestBrCliListByParentWithNoChildren(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{"dep list ep-1": `[]`})

	beads, err := NewBrCliBackend(repo).List(&BeadListFilters{Parent: "ep-1"}, repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(beads) != 0 {
		t.Errorf("expected no children, got %d", len(beads))
	}
}

// br list wraps its results in an object, unlike show and dep list. Decoding it
// as an array yields zero beads and no error.
func TestBrCliListUnwrapsTheObjectEnvelope(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"list": `{"issues":[{"id":"ep-1","title":"an epic","issue_type":"epic","status":"open"}],"total":1}`,
	})

	beads, err := NewBrCliBackend(repo).List(&BeadListFilters{Type: "epic"}, repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(beads) != 1 || beads[0].ID != "ep-1" {
		t.Fatalf("got %+v", beads)
	}
	// --limit defaults to 50, which would drop the tail of any larger epic.
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "--limit 0") {
		t.Error("list must ask for every row, not br's default page of 50")
	}
}

func TestBrCliUpdateStatus(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{"update kb-1": `[{"id":"kb-1","status":"implementation"}]`})

	if err := NewBrCliBackend(repo).Update("kb-1", UpdateBeadInput{State: "implementation"}, repo); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "--status implementation") {
		t.Errorf("update must set the status, calls: %v", fake.calledWith())
	}
}

func TestBrCliUpdateWithNothingToChangeFailsLoud(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, nil)

	err := NewBrCliBackend(repo).Update("kb-1", UpdateBeadInput{}, repo)
	if err == nil {
		t.Fatal("an update that changes nothing must fail rather than report success")
	}
}

// br has no set-labels, so replacing the wf:state:* set means removing what is
// there and adding what was asked for.
func TestBrCliSetLabelsReplacesRatherThanAdds(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"update kb-1": `[{"id":"kb-1"}]`,
		"show kb-1":   `[{"id":"kb-1","labels":["wf:state:implementation","wf:profile:worker"]}]`,
	})

	err := NewBrCliBackend(repo).Update("kb-1", UpdateBeadInput{
		State:     "implementation_review",
		SetLabels: []string{"wf:profile:worker", "wf:state:implementation_review"},
	}, repo)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	joined := strings.Join(fake.calledWith(), "\n")
	if !strings.Contains(joined, "label remove kb-1 wf:state:implementation") {
		t.Errorf("the stale state label must be removed, calls:\n%s", joined)
	}
	if !strings.Contains(joined, "label add kb-1 wf:state:implementation_review") {
		t.Errorf("the new state label must be added, calls:\n%s", joined)
	}
	if strings.Contains(joined, "label remove kb-1 wf:profile:worker") {
		t.Errorf("a label that is still wanted must not be removed, calls:\n%s", joined)
	}
}

func TestBrCliClose(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"close kb-1": `[{"id":"kb-1","status":"closed","close_reason":"shipped"}]`,
	})

	state, err := NewBrCliBackend(repo).Close("kb-1", "shipped", repo)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if state.State != "closed" || state.Reason != "shipped" {
		t.Errorf("got %+v", state)
	}
}

// The port names the blocker first; br names the dependent first.
func TestBrCliAddDependencyPassesTheDependentFirst(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{"dep add": `{"status":"ok"}`})

	if err := NewBrCliBackend(repo).AddDependency("kb-1", "kb-2", repo); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "dep add kb-2 kb-1") {
		t.Errorf("kb-2 depends on kb-1, so br must be called as `dep add kb-2 kb-1`, calls: %v", fake.calledWith())
	}
}

func TestBrCliComment(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{"comments add": `{"id":1}`})

	if err := NewBrCliBackend(repo).Comment("kb-1", "stage done", repo); err != nil {
		t.Fatalf("Comment: %v", err)
	}
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "comments add kb-1 stage done") {
		t.Errorf("calls: %v", fake.calledWith())
	}
}

// br reports failures as a JSON envelope on stdout. A caller reading only the
// exit code loses the reason, and one reading only stdout as data decodes an
// error as an empty result.
func TestBrCliSurfacesBrsErrorEnvelope(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"show nope": `{"error":{"code":"ISSUE_NOT_FOUND","message":"Issue not found: nope","hint":"Run 'br list' to see available issues."}}`,
	})

	_, err := NewBrCliBackend(repo).Get("nope", repo)
	if err == nil {
		t.Fatal("expected br's error to surface rather than an empty result")
	}
	for _, want := range []string{"ISSUE_NOT_FOUND", "Issue not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must carry br's own reason %q, got: %v", want, err)
		}
	}
}

func TestBrCliUnimplementedMethodsFailLoud(t *testing.T) {
	repo := brRepo(t)
	be := NewBrCliBackend(repo)

	if _, err := be.Create(CreateBeadInput{Title: "x"}, repo); err == nil {
		t.Error("create must fail loud rather than pretend")
	}
	if _, err := be.Query("status = open", nil, repo); err == nil {
		t.Error("query must fail loud rather than return nothing")
	}
	if err := be.Delete("kb-1", repo); err == nil {
		t.Error("delete must fail loud")
	}
}

// The adapter's job is to hand NormalizeBead the shape it already knows, so
// the mapping is asserted directly rather than through a round trip.
func TestBrIssueToRawBead(t *testing.T) {
	var issue brIssue
	raw := `{"id":"kb-1","title":"t","description":"d","notes":"n","acceptance_criteria":"ac",
	         "status":"implementation","priority":1,"issue_type":"bug","labels":["l"],"parent":"ep-1",
	         "dependencies":[{"id":"kb-0","dependency_type":"blocks"}]}`
	if err := json.Unmarshal([]byte(raw), &issue); err != nil {
		t.Fatal(err)
	}

	got := issue.toRawBead()
	if got.AcceptanceCriteria != "ac" {
		t.Errorf("acceptance_criteria must survive the mapping, got %q", got.AcceptanceCriteria)
	}
	if got.Parent != "ep-1" {
		t.Errorf("parent = %q", got.Parent)
	}
	if len(got.Dependencies) != 1 || got.Dependencies[0].SourceID != "kb-1" || got.Dependencies[0].TargetID != "kb-0" {
		t.Errorf("dependencies = %+v", got.Dependencies)
	}
}
