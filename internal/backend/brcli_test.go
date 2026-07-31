package backend

import (
	"encoding/json"
	"errors"
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

// The invocation goes into prompt text an agent reads and types, so it is
// shell syntax rather than argv. An unquoted path with a space renders as two
// arguments and the tracker tries to run the second word as a subcommand.
func TestTrackerInvocationQuotesThePath(t *testing.T) {
	t.Run("br pins a quoted database", func(t *testing.T) {
		parent := t.TempDir()
		repo := filepath.Join(parent, "kernl br review")
		if err := os.MkdirAll(filepath.Join(repo, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, ".beads", "beads.db"), []byte("sqlite"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := TrackerInvocation(MemoryManagerBeadsRust, repo)
		if err != nil {
			t.Fatalf("TrackerInvocation: %v", err)
		}
		want := "br --db '" + filepath.Join(repo, ".beads", "beads.db") + "'"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("bd pins a quoted repo", func(t *testing.T) {
		got, err := TrackerInvocation(MemoryManagerBeads, "/tmp/a repo")
		if err != nil {
			t.Fatalf("TrackerInvocation: %v", err)
		}
		if got != "bd -C '/tmp/a repo'" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("a quote in the path cannot escape the quoting", func(t *testing.T) {
		got, err := TrackerInvocation(MemoryManagerBeads, `/tmp/it's here`)
		if err != nil {
			t.Fatalf("TrackerInvocation: %v", err)
		}
		if got != `bd -C '/tmp/it'\''s here'` {
			t.Errorf("got %q", got)
		}
	})
}

// Both stores inside one .beads/ is a real state - running the wrong tracker
// once leaves it behind - and picking whichever is visited first means reading
// a database nobody chose.
func TestDetectMemoryManagerDeclinesWhenBothStoresArePresent(t *testing.T) {
	repo := brRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".beads", "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := DetectMemoryManager(repo); got != "" {
		t.Errorf("detection must decline when both stores are present, got %q", got)
	}
	if _, err := ResolveMemoryManager(repo, ""); err == nil {
		t.Fatal("an ambiguous repository must fail loud rather than pick one")
	}
	// Declared wins, so the operator can still say which.
	got, err := ResolveMemoryManager(repo, "br")
	if err != nil || got != MemoryManagerBeadsRust {
		t.Errorf("a declared tracker must settle it, got %q / %v", got, err)
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
	if !strings.Contains(joined, "--direction=up") {
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
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "--limit=0") {
		t.Error("list must ask for every row, not br's default page of 50")
	}
}

func TestBrCliUpdateStatus(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{"update kb-1": `[{"id":"kb-1","status":"implementation"}]`})

	if err := NewBrCliBackend(repo).Update("kb-1", UpdateBeadInput{State: "implementation"}, repo); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "--status=implementation") {
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

// Rewind used to be brUnimplemented against every repository this
// orchestrator actually drives (br, not knots) - the composite
// revert-decision verb's "rewind" half would have failed loud on every real
// run. This proves it now runs the same br update invocation MarkTerminal
// already uses in production for the status change.
func TestBrCliRewindUpdatesStatus(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"show kb-1":   `[{"id":"kb-1","status":"implementation"}]`,
		"update kb-1": `[{"id":"kb-1","status":"ready_for_implementation"}]`,
	})

	if err := NewBrCliBackend(repo).Rewind("kb-1", "ready_for_implementation", "wrong dependency chosen", repo); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	calls := strings.Join(fake.calledWith(), "\n")
	if !strings.Contains(calls, "--status=ready_for_implementation") {
		t.Errorf("rewind must set the target status, calls: %v", fake.calledWith())
	}
	if !strings.Contains(calls, "--notes=wrong dependency chosen") {
		t.Errorf("rewind must record the reason as notes, calls: %v", fake.calledWith())
	}
}

// A bare `--notes <reason>` write replaces the field wholesale (same as
// --description and --acceptance), and invariants for a br-backed bead live
// entirely inside that one free-text field as an [Invariants] block. A
// rewind - the mechanism revert-decision uses to reopen a bead - must not be
// the thing that silently erases the scope/state invariants already
// governing that bead; that is exactly the kind of silent constraint loss
// this feature exists to prevent, delivered by the feature itself.
func TestBrCliRewindPreservesInvariantsInNotes(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"show kb-1": `[{"id":"kb-1","status":"implementation","notes":"Existing prose note.\n\n[Invariants]\nScope: internal/api"}]`,
	})

	if err := NewBrCliBackend(repo).Rewind("kb-1", "ready_for_implementation", "wrong dependency chosen", repo); err != nil {
		t.Fatalf("Rewind: %v", err)
	}
	// These three strings only ever appear as argv of the update call (the
	// show call's args are just "show kb-1"), so a joined-log substring
	// check unambiguously exercises what was sent to update, matching this
	// file's existing convention for multi-call assertions.
	calls := strings.Join(fake.calledWith(), "\n")
	if !strings.Contains(calls, "Existing prose note.") {
		t.Errorf("rewind dropped the existing prose note, calls: %q", calls)
	}
	if !strings.Contains(calls, "[Invariants]") || !strings.Contains(calls, "Scope: internal/api") {
		t.Errorf("rewind dropped the invariants block, calls: %q", calls)
	}
	if !strings.Contains(calls, "wrong dependency chosen") {
		t.Errorf("rewind dropped the reason, calls: %q", calls)
	}
}

// A rewind target that is not a queue state (ready_for_*) leaves the bead in
// an active state nothing ever dispatches from - a rewind that "succeeds"
// into a stuck bead. KnotsBackend.Rewind already refuses this; br must not be
// the one backend that lets it through.
func TestBrCliRewindRejectsNonQueueTarget(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, nil)

	err := NewBrCliBackend(repo).Rewind("kb-1", "implementation", "reason", repo)
	if err == nil {
		t.Fatal("rewind to a non ready_for_* state must fail loud")
	}
	if !strings.Contains(err.Error(), "KERNL WORKFLOW CORRECTION FAILURE") {
		t.Errorf("error must carry the fail-loud marker, got: %v", err)
	}
}

// Replacing the wf:state:* set travels with the status change it mirrors.
// Doing it afterwards as remove-then-add was not atomic: a failure partway left
// a new status beside a half-replaced label set, which the workflow then reads
// as truth.
func TestBrCliSetLabelsTravelsWithTheStatusChange(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{"update kb-1": `[{"id":"kb-1"}]`})

	err := NewBrCliBackend(repo).Update("kb-1", UpdateBeadInput{
		State:     "implementation_review",
		SetLabels: []string{"wf:profile:worker", "wf:state:implementation_review"},
	}, repo)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	calls := fake.calledWith()
	if len(calls) != 1 {
		t.Fatalf("the status and its labels must be one command, got %d:\n%s", len(calls), strings.Join(calls, "\n"))
	}
	for _, want := range []string{
		"--status=implementation_review",
		"--set-labels=wf:profile:worker",
		"--set-labels=wf:state:implementation_review",
	} {
		if !strings.Contains(calls[0], want) {
			t.Errorf("call must carry %q, got:\n%s", want, calls[0])
		}
	}
	if strings.Contains(calls[0], "label remove") {
		t.Error("--set-labels replaces the whole set; nothing needs removing by hand")
	}
}

// An update that only replaces labels is a real update. The no-change guard
// used to run before SetLabels was considered, so `epic run` relabelling an
// epic was refused outright.
func TestBrCliUpdateWithOnlyLabelsIsAnUpdate(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{"update kb-1": `[{"id":"kb-1"}]`})

	if err := NewBrCliBackend(repo).Update("kb-1", UpdateBeadInput{SetLabels: []string{"wf:profile:epic"}}, repo); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "--set-labels=wf:profile:epic") {
		t.Errorf("calls: %v", fake.calledWith())
	}
}

// br emits a priority on every issue and 0 is a real P0. The shared normalizer
// maps 0 to 2 because for bd an absent key decodes as 0 and means "unset", so
// passing br's value through unchanged quietly demoted every P0 to P2.
func TestBrCliKeepsP0(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"show kb-1": `[{"id":"kb-1","title":"urgent","status":"open","priority":0}]`,
	})

	bead, err := NewBrCliBackend(repo).Get("kb-1", repo)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if bead.Priority != 0 {
		t.Errorf("Priority = %d, want the P0 br reported", bead.Priority)
	}
}

// An issue br says nothing about keeps the shared default, which is what the
// pointer exists to tell apart.
func TestBrCliAbsentPriorityKeepsTheDefault(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"show kb-1": `[{"id":"kb-1","title":"no priority","status":"open"}]`,
	})

	bead, err := NewBrCliBackend(repo).Get("kb-1", repo)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if bead.Priority != 2 {
		t.Errorf("Priority = %d, want the default 2", bead.Priority)
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

// Create is what Phase 6's fix-up beads need. `br create --json` prints one
// issue object, not an array - unlike show/list/dep-list - and ParentID maps
// directly onto --parent, which a live `br create --help` (and a throwaway
// workspace) confirmed exists and creates a real parent-child dependency.
func TestBrCliCreate(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"create --type=bug": `{"id":"kb-new-1","title":"Fix the thing","status":"open","priority":1,"issue_type":"bug","labels":["kernl:fixup"],"parent":"ep-1"}`,
	})

	bead, err := NewBrCliBackend(repo).Create(CreateBeadInput{
		Title:       "Fix the thing",
		Description: "it is broken",
		Type:        "bug",
		Priority:    1,
		Labels:      []string{"kernl:fixup"},
		ParentID:    "ep-1",
	}, repo)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if bead.ID != "kb-new-1" || bead.Type != "bug" || bead.ParentID != "ep-1" || bead.Priority != 1 {
		t.Errorf("got %+v", bead)
	}

	joined := strings.Join(fake.calledWith(), "\n")
	for _, want := range []string{"-- Fix the thing", "--type=bug", "--priority=1", "--description=it is broken", "--labels=kernl:fixup", "--parent=ep-1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("calls %v must contain %q", fake.calledWith(), want)
		}
	}
}

// br create has no --acceptance or --notes flag at all (confirmed against a
// live `br create --help` and a live rejected call, exit 2) - unlike br
// update, which has both. So a caller asking for either gets a second,
// immediate update call, not a silently dropped field.
func TestBrCliCreateWritesAcceptanceAndNotesAsASecondUpdate(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"create --priority=0 -- Bare title": `{"id":"kb-new-2","title":"Bare title","status":"open","priority":2,"issue_type":"task"}`,
		"update kb-new-2":                   `[{"id":"kb-new-2","status":"open"}]`,
	})

	bead, err := NewBrCliBackend(repo).Create(CreateBeadInput{
		Title:      "Bare title",
		Acceptance: "must do X",
		Notes:      "some notes",
	}, repo)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if bead.Acceptance != "must do X" || bead.Notes != "some notes" {
		t.Errorf("got %+v", bead)
	}

	joined := strings.Join(fake.calledWith(), "\n")
	if !strings.Contains(joined, "update kb-new-2 --acceptance=must do X --notes=some notes") {
		t.Errorf("acceptance/notes must arrive as a second update call, calls: %v", fake.calledWith())
	}
}

// br has no separate storage for invariants - they live entirely inside the
// notes field (see embedInvariantsInNotes) - so Create must embed them the
// same way Rewind already does, not drop them because create itself cannot
// carry them directly.
func TestBrCliCreateEmbedsInvariantsIntoNotes(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"create --priority=0 -- Inv title": `{"id":"kb-new-3","title":"Inv title","status":"open","priority":2,"issue_type":"task"}`,
		"update kb-new-3":                  `[{"id":"kb-new-3","status":"open"}]`,
	})

	if _, err := NewBrCliBackend(repo).Create(CreateBeadInput{
		Title:      "Inv title",
		Invariants: []Invariant{{Kind: "must", Condition: "never delete X"}},
	}, repo); err != nil {
		t.Fatalf("Create: %v", err)
	}

	joined := strings.Join(fake.calledWith(), "\n")
	if !strings.Contains(joined, "must: never delete X") {
		t.Errorf("invariants must be embedded into the notes update, calls: %v", fake.calledWith())
	}
}

// ProfileID/WorkflowID have no br create equivalent, not even a two-step
// one - an accepted-and-discarded field is worse than a refused one.
func TestBrCliCreateRejectsProfileAndWorkflowID(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{})

	if _, err := NewBrCliBackend(repo).Create(CreateBeadInput{Title: "x", ProfileID: "worker"}, repo); err == nil {
		t.Fatal("ProfileID must be refused, not silently dropped")
	}
	if _, err := NewBrCliBackend(repo).Create(CreateBeadInput{Title: "x", WorkflowID: "wf"}, repo); err == nil {
		t.Fatal("WorkflowID must be refused, not silently dropped")
	}
}

func TestBrCliCreateRequiresTitle(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{})
	if _, err := NewBrCliBackend(repo).Create(CreateBeadInput{}, repo); err == nil {
		t.Fatal("an empty title must be refused")
	}
}

// Create used to put the title right after "create", ahead of every flag -
// the one value in this function brValue cannot protect, since the title is
// positional, not a flag's own value. A title beginning with "-" (POST
// /api/beads accepts any string a caller sends, and this surface was
// unreachable before Phase 6 made Create callable at all) would then be
// read by br as the next flag instead of a title. Confirmed against the
// real `br 0.2.10` binary: `br --db <db> --json create --type task
// --priority=0 -- --help` creates an issue literally titled "--help"
// (exit 0), which is exactly what this test pins in the argv shape.
func TestBrCliCreateTitleStartingWithDashIsNotReadAsAFlag(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"create --priority=0 -- --help": `{"id":"kb-new-4","title":"--help","status":"open","priority":0,"issue_type":"task"}`,
	})

	bead, err := NewBrCliBackend(repo).Create(CreateBeadInput{Title: "--help"}, repo)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if bead.Title != "--help" {
		t.Errorf("got %+v, want a bead literally titled \"--help\"", bead)
	}

	calls := fake.calledWith()
	if len(calls) != 1 {
		t.Fatalf("expected exactly one br call, got %v", calls)
	}
	// The title must be the LAST argument, after every flag and after the
	// "--" boundary that ends flag parsing - anywhere else, br would read
	// "--help" as a request for its own help text instead of a title.
	if !strings.HasSuffix(calls[0], "-- --help") {
		t.Errorf("call %q must end with the title after a `--` boundary", calls[0])
	}
}

// A failure between "the issue was created" and "its acceptance/notes were
// written" must not discard the created bead's id - a caller told nothing
// was created would call Create again for the same title, producing two
// issues for one request.
func TestBrCliCreatePartialFailureCarriesTheCreatedBead(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"create --priority=0 -- Partial title": `{"id":"kb-new-5","title":"Partial title","status":"open","priority":0,"issue_type":"task"}`,
		"update kb-new-5":                      `{"error":{"code":"LOCKED","message":"database is locked"}}`,
	})

	_, err := NewBrCliBackend(repo).Create(CreateBeadInput{
		Title:      "Partial title",
		Acceptance: "must do X",
	}, repo)
	if err == nil {
		t.Fatal("expected the acceptance update failure to surface")
	}

	var partial *CreatePartialError
	if !errors.As(err, &partial) {
		t.Fatalf("expected a *CreatePartialError so the caller can recover the created bead, got: %v", err)
	}
	if partial.Bead == nil || partial.Bead.ID != "kb-new-5" {
		t.Errorf("CreatePartialError.Bead = %+v, want the bead that was actually created", partial.Bead)
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
	if !strings.Contains(strings.Join(fake.calledWith(), "\n"), "comments add kb-1 -- stage done") {
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

// br's parser reads a value beginning with "-" as the next flag and exits 2,
// so a bead whose description starts with a dash - or a comment body that does
// - would abort the stage rather than be recorded.
func TestBrCliPassesValuesThatLookLikeFlags(t *testing.T) {
	repo := brRepo(t)
	fake := newFakeBr(t, map[string]string{
		"update kb-1":  `[{"id":"kb-1"}]`,
		"comments add": `{"id":1}`,
	})
	be := NewBrCliBackend(repo)

	if err := be.Update("kb-1", UpdateBeadInput{Notes: "--dash notes"}, repo); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := be.Comment("kb-1", "--force is a comment", repo); err != nil {
		t.Fatalf("Comment: %v", err)
	}

	joined := strings.Join(fake.calledWith(), "\n")
	if !strings.Contains(joined, "--notes=--dash notes") {
		t.Errorf("a flag value must be passed as --flag=value, calls:\n%s", joined)
	}
	if !strings.Contains(joined, "comments add kb-1 -- --force is a comment") {
		t.Errorf("a positional value must be passed after --, calls:\n%s", joined)
	}
}

// br can print two JSON documents back to back: `close` on an already-closed
// issue emits its per-issue result and then the error envelope. Decoding the
// whole buffer at once fails on the trailing document, so the envelope went
// unseen and the caller got a bare exit status with an empty stderr.
func TestBrCliFindsAnErrorEnvelopeAfterAResult(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"close kb-1": `{"closed":[],"skipped":[{"id":"kb-1","reason":"already closed"}]}
{"error":{"code":"NOTHING_TO_DO","message":"Nothing to do: all 1 issue(s) skipped","hint":"All specified issues were already closed or not found."}}`,
	})

	_, err := NewBrCliBackend(repo).Close("kb-1", "shipped", repo)
	if err == nil {
		t.Fatal("expected br's reason to surface")
	}
	if !strings.Contains(err.Error(), "NOTHING_TO_DO") {
		t.Errorf("error must carry br's own code, got: %v", err)
	}
}

// A result that merely mentions an error is not a failure; only a document
// whose top level is {"error": {...}} is.
func TestBrCliDoesNotMistakeAnErrorFieldForAFailure(t *testing.T) {
	repo := brRepo(t)
	newFakeBr(t, map[string]string{
		"show kb-1": `[{"id":"kb-1","title":"fix the error envelope","status":"open"}]`,
	})

	bead, err := NewBrCliBackend(repo).Get("kb-1", repo)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if bead.Title != "fix the error envelope" {
		t.Errorf("got %+v", bead)
	}
}

func TestBrCliUnimplementedMethodsFailLoud(t *testing.T) {
	repo := brRepo(t)
	be := NewBrCliBackend(repo)

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
