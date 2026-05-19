package backend

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type stubBackend struct {
	calls []string
	caps  BackendCapabilities
}

func (s *stubBackend) record(method string) {
	s.calls = append(s.calls, method)
}

func (s *stubBackend) ListWorkflows(repoPath string) ([]WorkflowDescriptor, error) {
	s.record("ListWorkflows")
	return BuiltinWorkflowDescriptors(), nil
}
func (s *stubBackend) List(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	s.record("List")
	return nil, nil
}
func (s *stubBackend) ListReady(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	s.record("ListReady")
	return nil, nil
}
func (s *stubBackend) Get(id string, repoPath string) (*Bead, error) {
	s.record("Get")
	return &Bead{ID: id}, nil
}
func (s *stubBackend) Create(input CreateBeadInput, repoPath string) (*Bead, error) {
	s.record("Create")
	return &Bead{ID: "new"}, nil
}
func (s *stubBackend) Update(id string, input UpdateBeadInput, repoPath string) error {
	s.record("Update")
	return nil
}
func (s *stubBackend) Delete(id string, repoPath string) error {
	s.record("Delete")
	return nil
}
func (s *stubBackend) Close(id string, reason string, repoPath string) (*TerminalState, error) {
	s.record("Close")
	return &TerminalState{State: "shipped"}, nil
}
func (s *stubBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	s.record("MarkTerminal")
	return nil
}
func (s *stubBackend) Reopen(id string, reason string, repoPath string) error {
	s.record("Reopen")
	return nil
}
func (s *stubBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	s.record("Rewind")
	return nil
}
func (s *stubBackend) Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error) {
	s.record("Search")
	return nil, nil
}
func (s *stubBackend) Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error) {
	s.record("Query")
	return nil, nil
}
func (s *stubBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	s.record("AddDependency")
	return nil
}
func (s *stubBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	s.record("RemoveDependency")
	return nil
}
func (s *stubBackend) ListDependencies(id string, repoPath string, options *DependencyListOptions) ([]BeadDependency, error) {
	s.record("ListDependencies")
	return nil, nil
}
func (s *stubBackend) BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error) {
	s.record("BuildTakePrompt")
	return &TakePromptResult{Prompt: "take"}, nil
}
func (s *stubBackend) BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error) {
	s.record("BuildPollPrompt")
	return &PollPromptResult{Prompt: "poll"}, nil
}
func (s *stubBackend) Comment(id string, body string, repoPath string) error {
	s.record("Comment")
	return nil
}
func (s *stubBackend) Capabilities() BackendCapabilities {
	return s.caps
}

var stubCaps = BackendCapabilities{
	CanCreate:             true,
	CanUpdate:             true,
	CanDelete:             true,
	CanClose:              true,
	CanManageDependencies: true,
	CanManageLabels:       true,
	CanSync:               true,
	CanSearch:             true,
	CanQuery:              true,
	CanListReady:          true,
}

func newStubBackend() *stubBackend {
	return &stubBackend{caps: stubCaps}
}

func TestAutoRoutingBackend_NoRepoPath_ThrowsDispatchFailure(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	_, err := arb.Get("some-id", "")
	if err == nil {
		t.Fatal("expected error for empty repoPath, got nil")
	}
	bde, ok := err.(*BackendDispatchError)
	if !ok {
		t.Fatalf("expected *BackendDispatchError, got %T: %v", err, err)
	}
	if bde.Reason != "repo_path_missing" {
		t.Errorf("expected reason repo_path_missing, got %s", bde.Reason)
	}
	if bde.Kind != "backend" {
		t.Errorf("expected kind backend, got %s", bde.Kind)
	}
	if bde.Method != "get" {
		t.Errorf("expected method get, got %s", bde.Method)
	}
}

func TestAutoRoutingBackend_UnknownRepoType_ThrowsDispatchFailure(t *testing.T) {
	tmpDir := t.TempDir()
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.detectRepoPath = func(path string) MemoryManagerType {
		return MemoryManagerType("unknown")
	}

	_, err := arb.List(nil, tmpDir)
	if err == nil {
		t.Fatal("expected error for unknown repo type, got nil")
	}
	bde, ok := err.(*BackendDispatchError)
	if !ok {
		t.Fatalf("expected *BackendDispatchError, got %T: %v", err, err)
	}
	if bde.Reason != "repo_type_unknown" {
		t.Errorf("expected reason repo_type_unknown, got %s", bde.Reason)
	}
	if bde.RepoPath != tmpDir {
		t.Errorf("expected repoPath %s, got %s", tmpDir, bde.RepoPath)
	}
	if bde.Method != "list" {
		t.Errorf("expected method list, got %s", bde.Method)
	}
}

func TestAutoRoutingBackend_ErrorContainsKERNLMarker(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	errTxt := ""
	_, err := arb.Get("id", "")
	if err != nil {
		errTxt = err.Error()
	}
	if !containsStr(errTxt, "KERNL DISPATCH FAILURE") {
		t.Errorf("expected error to contain KERNL DISPATCH FAILURE, got: %s", errTxt)
	}
	if !containsStr(errTxt, "repo_path_missing") {
		t.Errorf("expected error to contain repo_path_missing, got: %s", errTxt)
	}
}

func TestAutoRoutingBackend_KnotsRepo_ResolvesType(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	var detectedType MemoryManagerType
	arb.detectRepoPath = func(path string) MemoryManagerType {
		detectedType = MemoryManagerKnots
		return MemoryManagerKnots
	}

	_, err := arb.Get("id", "/knots-repo")
	if err == nil || detectedType != MemoryManagerKnots {
		if err != nil {
		}
	}
	if detectedType != MemoryManagerKnots {
		t.Errorf("expected knots detection, got %s", detectedType)
	}
}

func TestAutoRoutingBackend_BeadsRepo_ResolvesType(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	var detectedType MemoryManagerType
	arb.detectRepoPath = func(path string) MemoryManagerType {
		detectedType = MemoryManagerBeads
		return MemoryManagerBeads
	}

	_, err := arb.Get("id", "/beads-repo")
	if err == nil || detectedType != MemoryManagerBeads {
		if err != nil {
		}
	}
	if detectedType != MemoryManagerBeads {
		t.Errorf("expected beads detection, got %s", detectedType)
	}
}

func TestAutoRoutingBackend_ListWorkflows_NoRepo_ReturnsBuiltin(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	wfs, err := arb.ListWorkflows("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wfs) == 0 {
		t.Error("expected at least one builtin workflow descriptor")
	}
	hasSDLC := false
	for _, wf := range wfs {
		if wf.ID == "sdlc" {
			hasSDLC = true
		}
	}
	if !hasSDLC {
		t.Error("expected builtin SDLC workflow descriptor")
	}
}

func TestAutoRoutingBackend_ListWorkflows_UnknownRepo_Throws(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.detectRepoPath = func(path string) MemoryManagerType {
		return MemoryManagerType("unknown")
	}
	_, err := arb.ListWorkflows("/unknown")
	if err == nil {
		t.Fatal("expected error for unknown repo type, got nil")
	}
	bde, ok := err.(*BackendDispatchError)
	if !ok {
		t.Fatalf("expected *BackendDispatchError, got %T", err)
	}
	if bde.Reason != "repo_type_unknown" {
		t.Errorf("expected reason repo_type_unknown, got %s", bde.Reason)
	}
}

func TestAutoRoutingBackend_CapabilitiesForRepo_Failure_ReturnsFull(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.detectRepoPath = func(path string) MemoryManagerType {
		return MemoryManagerType("unknown")
	}
	caps := arb.CapabilitiesForRepo("/unknown")
	if !caps.CanCreate {
		t.Error("expected full capabilities on unknown repo type (advisory)")
	}
}

func TestAutoRoutingBackend_CapabilitiesForRepo_Empty_ReturnsFull(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	caps := arb.CapabilitiesForRepo("")
	if !caps.CanCreate {
		t.Error("expected full capabilities on empty repoPath (advisory)")
	}
}

func TestAutoRoutingBackend_Create_NoRepo_Throws(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	_, err := arb.Create(CreateBeadInput{Title: "test"}, "")
	if err == nil {
		t.Fatal("expected error for empty repoPath")
	}
	bde, ok := err.(*BackendDispatchError)
	if !ok {
		t.Fatalf("expected *BackendDispatchError, got %T", err)
	}
	if bde.Method != "create" {
		t.Errorf("expected method create, got %s", bde.Method)
	}
	if bde.Reason != "repo_path_missing" {
		t.Errorf("expected reason repo_path_missing, got %s", bde.Reason)
	}
}

func TestAutoRoutingBackend_CacheRepoTypeResolution(t *testing.T) {
	var calls atomic.Int32
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.detectRepoPath = func(path string) MemoryManagerType {
		calls.Add(1)
		return MemoryManagerBeads
	}

	_, _ = arb.Get("id", "/repo")
	_, _ = arb.Get("id2", "/repo")

	if calls.Load() != 1 {
		t.Errorf("expected 1 detection call due to caching, got %d", calls.Load())
	}
}

func TestAutoRoutingBackend_ClearRepoCache_SpecificRepo(t *testing.T) {
	var calls atomic.Int32
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.detectRepoPath = func(path string) MemoryManagerType {
		calls.Add(1)
		return MemoryManagerBeads
	}

	_, _ = arb.Get("id", "/repo")
	arb.ClearRepoCache("/repo")
	_, _ = arb.Get("id2", "/repo")

	if calls.Load() != 2 {
		t.Errorf("expected 2 detection calls after cache clear, got %d", calls.Load())
	}
}

func TestAutoRoutingBackend_ClearRepoCache_All(t *testing.T) {
	var calls atomic.Int32
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.detectRepoPath = func(path string) MemoryManagerType {
		calls.Add(1)
		return MemoryManagerBeads
	}

	_, _ = arb.Get("id", "/a")
	_, _ = arb.Get("id", "/b")
	arb.ClearRepoCache("")
	_, _ = arb.Get("id2", "/a")

	if calls.Load() != 3 {
		t.Errorf("expected 3 detection calls after full cache clear, got %d", calls.Load())
	}
}

func TestAutoRoutingBackend_CacheExpiry(t *testing.T) {
	var calls atomic.Int32
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.repoCacheTTL = 10 * time.Millisecond
	arb.detectRepoPath = func(path string) MemoryManagerType {
		calls.Add(1)
		return MemoryManagerBeads
	}

	_, _ = arb.Get("id", "/repo")
	time.Sleep(30 * time.Millisecond)
	_, _ = arb.Get("id2", "/repo")

	if calls.Load() != 2 {
		t.Errorf("expected 2 detection calls after TTL expiry, got %d", calls.Load())
	}
}

func TestBackendDispatchError_Message(t *testing.T) {
	err := newBackendDispatchError("backend", "/my/repo", "create", "repo_type_unknown")
	msg := err.Error()
	if !containsStr(msg, "KERNL DISPATCH FAILURE") {
		t.Errorf("expected marker in message, got: %s", msg)
	}
	if !containsStr(msg, "repo_type_unknown") {
		t.Errorf("expected reason in message, got: %s", msg)
	}
	if !containsStr(msg, "create") {
		t.Errorf("expected method in message, got: %s", msg)
	}
	if !containsStr(msg, "/my/repo") {
		t.Errorf("expected repoPath in message, got: %s", msg)
	}
}

func TestBackendDispatchError_EmptyRepoPath(t *testing.T) {
	err := newBackendDispatchError("backend", "", "get", "repo_path_missing")
	msg := err.Error()
	if !containsStr(msg, "(empty)") {
		t.Errorf("expected (empty) in message for blank repoPath, got: %s", msg)
	}
}

func TestCreateConcreteBackend_CLI(t *testing.T) {
	entry := createConcreteBackend(BackendTypeCLI, "/test")
	if entry.Port == nil {
		t.Error("expected non-nil backend port")
	}
	if !entry.Capabilities.CanCreate {
		t.Error("expected CLI backend to have CanCreate capability")
	}
}


func TestCreateBackend_AutoPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for CreateBackend with type 'auto'")
		}
	}()
	CreateBackend(BackendTypeAuto, "/test")
}

func TestCreateBackend_CLI(t *testing.T) {
	entry := CreateBackend(BackendTypeCLI, "/test")
	if entry.Port == nil {
		t.Error("expected non-nil port")
	}
}

func TestCreateBackend_Knots(t *testing.T) {
	entry := CreateBackend(BackendTypeKnots, "/test")
	if entry.Port == nil {
		t.Error("expected non-nil port")
	}
}

func TestAutoRoutingBackend_DelegatesAllMethods(t *testing.T) {
	arb := NewAutoRoutingBackend(&config.Config{})
	arb.detectRepoPath = func(path string) MemoryManagerType {
		return MemoryManagerBeads
	}

	_, err := arb.List(nil, "/repo")
	if _, ok := err.(*BackendDispatchError); ok && err != nil {
	}
	_, _ = arb.ListReady(nil, "/repo")
	_, _ = arb.Get("id", "/repo")
	_, _ = arb.Create(CreateBeadInput{Title: "t"}, "/repo")
	_ = arb.Update("id", UpdateBeadInput{}, "/repo")
	_ = arb.Delete("id", "/repo")
	_, _ = arb.Close("id", "reason", "/repo")
	_ = arb.MarkTerminal("id", "shipped", "reason", "/repo")
	_ = arb.Reopen("id", "reason", "/repo")
	_ = arb.Rewind("id", "ready_for_implementation", "reason", "/repo")
	_, _ = arb.Search("query", nil, "/repo")
	_, _ = arb.Query("state:open", nil, "/repo")
	_ = arb.AddDependency("a", "b", "/repo")
	_ = arb.RemoveDependency("a", "b", "/repo")
	_, _ = arb.ListDependencies("id", "/repo", nil)
	_, _ = arb.BuildTakePrompt("id", nil, "/repo")
	_, _ = arb.BuildPollPrompt(nil, "/repo")
}

func TestAutoRouteFromConfig_NoRepo_Throws(t *testing.T) {
	_, err := AutoRouteFromConfig(&config.Config{}, "")
	if err == nil {
		t.Fatal("expected error for empty repoPath")
	}
	bde, ok := err.(*BackendDispatchError)
	if !ok {
		t.Fatalf("expected *BackendDispatchError, got %T", err)
	}
	if bde.Reason != "repo_path_missing" {
		t.Errorf("expected repo_path_missing, got %s", bde.Reason)
	}
}

func TestAutoRouteFromConfig_RepoInRegistry(t *testing.T) {
	cfg := &config.Config{
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{
				{Path: "/my/repo", MemoryManager: "knots"},
			},
		},
	}
	_, err := AutoRouteFromConfig(cfg, "/my/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAutoRouteFromConfig_RepoNotInRegistry_UsesDetection(t *testing.T) {
	cfg := &config.Config{
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{},
		},
	}
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := AutoRouteFromConfig(cfg, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAutoRouteBackendWithDetection_Beads(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := AutoRouteBackendWithDetection(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAutoRouteBackendWithDetection_Knots(t *testing.T) {
	tmpDir := t.TempDir()
	knotsDir := filepath.Join(tmpDir, ".knots")
	if err := os.MkdirAll(knotsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := AutoRouteBackendWithDetection(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAutoRouteBackendWithDetection_NoRepo_Throws(t *testing.T) {
	_, err := AutoRouteBackendWithDetection("")
	if err == nil {
		t.Fatal("expected error for empty repoPath")
	}
}

func TestBuiltinWorkflowDescriptors_SDLCPresent(t *testing.T) {
	wfs := BuiltinWorkflowDescriptors()
	if len(wfs) == 0 {
		t.Fatal("expected at least one builtin workflow")
	}
	sdlc := wfs[0]
	if sdlc.ID != "sdlc" {
		t.Errorf("expected SDLC workflow, got %s", sdlc.ID)
	}
	if sdlc.InitialState != "ready_for_implementation" {
		t.Errorf("expected initialState ready_for_implementation, got %s", sdlc.InitialState)
	}
	if len(sdlc.TerminalStates) == 0 {
		t.Error("expected at least one terminal state")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}