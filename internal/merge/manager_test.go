package merge_test

import (
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/merge"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type child struct {
	ID     string
	Status workflow.IssueStatus
}

type fakeBackend struct {
	mu             sync.Mutex
	childrenByEpic map[string][]child
	updates        []string
	descByID       map[string]string
}

func (f *fakeBackend) ListChildrenAwaitingIntegration(epicID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.childrenByEpic[epicID] {
		if c.Status == workflow.StatusAwaitingIntegration {
			out = append(out, c.ID)
		}
	}
	return out, nil
}

func (f *fakeBackend) CountChildren(epicID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.childrenByEpic[epicID]), nil
}

func (f *fakeBackend) UpdateStatus(id string, s workflow.IssueStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, id+"->"+string(s))
	return nil
}

func (f *fakeBackend) UpdateState(id string, state string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, id+"->"+state)
	return nil
}

func (f *fakeBackend) GetDescription(id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.descByID[id], nil
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{childrenByEpic: map[string][]child{}, descByID: map[string]string{}}
}

type fakeDispatcher struct {
	mu     sync.Mutex
	spawns []string
}

func (d *fakeDispatcher) DispatchMerger(epicID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.spawns = append(d.spawns, epicID)
	return nil
}

func TestMergeManager_TriggerWhenAllChildrenAwaiting(t *testing.T) {
	b := newFakeBackend()
	b.childrenByEpic["e1"] = []child{
		{ID: "c1", Status: workflow.StatusAwaitingIntegration},
		{ID: "c2", Status: workflow.StatusAwaitingIntegration},
		{ID: "c3", Status: workflow.StatusAwaitingIntegration},
	}
	d := &fakeDispatcher{}
	m := merge.NewManager(b, d)
	if err := m.TryTrigger("e1"); err != nil {
		t.Fatal(err)
	}
	if len(d.spawns) != 1 || d.spawns[0] != "e1" {
		t.Fatalf("expected one spawn for e1, got %+v", d.spawns)
	}
}

func TestMergeManager_NoTriggerWhenSomeChildrenNotReady(t *testing.T) {
	b := newFakeBackend()
	b.childrenByEpic["e1"] = []child{
		{ID: "c1", Status: workflow.StatusAwaitingIntegration},
		{ID: "c2", Status: workflow.StatusInProgress},
	}
	d := &fakeDispatcher{}
	m := merge.NewManager(b, d)
	if err := m.TryTrigger("e1"); err != nil {
		t.Fatal(err)
	}
	if len(d.spawns) != 0 {
		t.Fatalf("expected no spawn, got %+v", d.spawns)
	}
}

func TestMergeManager_SingleFlight_100Goroutines(t *testing.T) {
	b := newFakeBackend()
	b.childrenByEpic["e1"] = []child{
		{ID: "c1", Status: workflow.StatusAwaitingIntegration},
		{ID: "c2", Status: workflow.StatusAwaitingIntegration},
	}
	d := &fakeDispatcher{}
	m := merge.NewManager(b, d)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.TryTrigger("e1")
		}()
	}
	wg.Wait()
	if len(d.spawns) != 1 {
		t.Fatalf("single-flight failure: %d spawns for e1", len(d.spawns))
	}
}

func TestMergeManager_RouteOutcome_Success(t *testing.T) {
	b := newFakeBackend()
	b.childrenByEpic["e1"] = []child{
		{ID: "c1", Status: workflow.StatusAwaitingIntegration},
	}
	b.descByID["e1"] = "epic_branch: feat/e1\nmerge_outcome: success\npr_url: https://x/pr/1\n"
	m := merge.NewManager(b, &fakeDispatcher{})
	if err := m.RouteOutcome("e1"); err != nil {
		t.Fatal(err)
	}
	got := b.updates
	want := []string{"e1->ready_for_integration"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestMergeManager_RouteOutcome_Blocked_Variants(t *testing.T) {
	cases := []struct {
		outcome string
		want    string
	}{
		{"merge_conflict", "e1->blocked"},
		{"push_failed", "e1->blocked"},
		{"pr_create_failed", "e1->blocked"},
	}
	for _, c := range cases {
		t.Run(c.outcome, func(t *testing.T) {
			b := newFakeBackend()
			b.childrenByEpic["e1"] = []child{{ID: "c1", Status: workflow.StatusAwaitingIntegration}}
			b.descByID["e1"] = "merge_outcome: " + c.outcome + "\n"
			m := merge.NewManager(b, &fakeDispatcher{})
			if err := m.RouteOutcome("e1"); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, u := range b.updates {
				if u == c.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected %q in %+v", c.want, b.updates)
			}
		})
	}
}

func TestMergeManager_RouteOutcome_PRAlreadyExists_AdoptsPR(t *testing.T) {
	b := newFakeBackend()
	b.childrenByEpic["e1"] = []child{{ID: "c1", Status: workflow.StatusAwaitingIntegration}}
	b.descByID["e1"] = "merge_outcome: pr_already_exists\npr_url: https://x/pr/42\n"
	m := merge.NewManager(b, &fakeDispatcher{})
	if err := m.RouteOutcome("e1"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range b.updates {
		if u == "e1->awaiting_pr_review" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected awaiting_pr_review, got %+v", b.updates)
	}
}

func TestMergeManager_RouteOutcome_MissingOutcome_Blocked(t *testing.T) {
	b := newFakeBackend()
	b.descByID["e1"] = "epic_branch: feat/e1\n"
	m := merge.NewManager(b, &fakeDispatcher{})
	if err := m.RouteOutcome("e1"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range b.updates {
		if u == "e1->blocked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocked, got %+v", b.updates)
	}
}

func TestMergeManager_RouteOutcome_InvalidOutcome_Blocked(t *testing.T) {
	b := newFakeBackend()
	b.descByID["e1"] = "merge_outcome: nonsense\n"
	m := merge.NewManager(b, &fakeDispatcher{})
	if err := m.RouteOutcome("e1"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range b.updates {
		if u == "e1->blocked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocked, got %+v", b.updates)
	}
}
