package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestRegistry(t *testing.T) (registryDir string, registryFile string) {
	t.Helper()
	registryDir = t.TempDir()
	registryFile = filepath.Join(registryDir, "registry.json")

	origDir := configDir
	origPath := registryFilePath

	configDir = func() string { return registryDir }
	registryFilePath = func() string { return registryFile }

	t.Cleanup(func() {
		configDir = origDir
		registryFilePath = origPath
	})

	return registryDir, registryFile
}

func writeTestRegistry(t *testing.T, path string, data map[string]any) {
	t.Helper()
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
}

func TestDetectMemoryManager_KnotsMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".knots"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := DetectMemoryManager(dir)
	if result != MemoryManagerKnots {
		t.Errorf("expected knots, got %s", result)
	}
}

// bd and br both store under .beads/, so the directory name identifies
// neither. Detecting on it returned bd for a br repository, and the run then
// opened a database that does not exist - which looks exactly like a tracker
// with no ready work.
func TestDetectMemoryManager_TellsBdAndBrApartByWhatIsInTheMarkerDirectory(t *testing.T) {
	t.Run("embeddeddolt is bd", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".beads", "embeddeddolt"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := DetectMemoryManager(dir); got != MemoryManagerBeads {
			t.Errorf("expected beads, got %q", got)
		}
	})

	t.Run("beads.db is br", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".beads", "beads.db"), []byte("sqlite"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := DetectMemoryManager(dir); got != MemoryManagerBeadsRust {
			t.Errorf("expected br, got %q", got)
		}
	})

	t.Run("an empty .beads names neither", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := DetectMemoryManager(dir); got != "" {
			t.Errorf("a marker directory that identifies no tracker must detect nothing, got %q", got)
		}
	})
}

func TestDetectMemoryManager_BothMarkers_KnotsWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".knots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	result := DetectMemoryManager(dir)
	if result != MemoryManagerKnots {
		t.Errorf("expected knots (higher precedence), got %s", result)
	}
}

func TestDetectMemoryManager_NoMarkerDetectsNothing(t *testing.T) {
	if got := DetectMemoryManager(t.TempDir()); got != "" {
		t.Errorf("a directory with no tracker store must detect nothing, got %q", got)
	}
}

// The rule the whole tracker question is settled by, in one place.
func TestResolveMemoryManager(t *testing.T) {
	brRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(brRepo, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brRepo, ".beads", "beads.db"), []byte("sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("the configured value wins over what is on disk", func(t *testing.T) {
		got, err := ResolveMemoryManager(brRepo, "beads")
		if err != nil {
			t.Fatalf("ResolveMemoryManager: %v", err)
		}
		if got != MemoryManagerBeads {
			t.Errorf("got %q, want the configured beads", got)
		}
	})

	t.Run("unset falls back to what is on disk", func(t *testing.T) {
		got, err := ResolveMemoryManager(brRepo, "")
		if err != nil {
			t.Fatalf("ResolveMemoryManager: %v", err)
		}
		if got != MemoryManagerBeadsRust {
			t.Errorf("got %q, want the detected br", got)
		}
	})

	t.Run("an unknown configured value fails loud", func(t *testing.T) {
		_, err := ResolveMemoryManager(brRepo, "beadz")
		if err == nil {
			t.Fatal("expected a refusal rather than a default")
		}
		if !strings.Contains(err.Error(), "memoryManager") {
			t.Errorf("the error must name the config key that fixes it, got: %v", err)
		}
	})

	t.Run("undetectable and unconfigured fails loud", func(t *testing.T) {
		_, err := ResolveMemoryManager(t.TempDir(), "")
		if err == nil {
			t.Fatal("expected a refusal rather than a guess")
		}
		if !strings.Contains(err.Error(), "memoryManager") {
			t.Errorf("the error must name the config key that fixes it, got: %v", err)
		}
	})
}

func TestIsKnownMemoryManagerType(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected bool
	}{
		{"knots", true},
		{"beads", true},
		{"unknown", false},
		{"", false},
	} {
		result := IsKnownMemoryManagerType(tc.input)
		if result != tc.expected {
			t.Errorf("IsKnownMemoryManagerType(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

func TestLoadRegistry_FileExists(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{
		"repos": []any{
			map[string]any{"path": "/tmp/repo1", "name": "repo1", "memoryManagerType": "knots"},
		},
	})

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(reg.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(reg.Repos))
	}
	if reg.Repos[0].Path != "/tmp/repo1" {
		t.Errorf("expected path /tmp/repo1, got %s", reg.Repos[0].Path)
	}
	if reg.Repos[0].MemoryManagerType != MemoryManagerKnots {
		t.Errorf("expected knots, got %s", reg.Repos[0].MemoryManagerType)
	}
}

func TestLoadRegistry_MissingFileReturnsEmpty(t *testing.T) {
	_, _ = setupTestRegistry(t)

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if reg.Repos != nil {
		t.Errorf("expected nil repos for missing file, got %v", reg.Repos)
	}
}

func TestLoadRegistry_InvalidJSONReturnsEmpty(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	if err := os.WriteFile(registryFile, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if reg.Repos != nil {
		t.Errorf("expected nil repos for invalid json, got %v", reg.Repos)
	}
}

func TestNormalizeRepo_SkipsEmptyPath(t *testing.T) {
	result := normalizeRepo(map[string]any{"path": ""})
	if result != nil {
		t.Errorf("expected nil for empty path, got %v", result)
	}
}

func TestNormalizeRepo_SkipsMissingPath(t *testing.T) {
	result := normalizeRepo(map[string]any{"name": "test"})
	if result != nil {
		t.Errorf("expected nil for missing path, got %v", result)
	}
}

func TestNormalizeRepo_DefaultsNameFromPath(t *testing.T) {
	result := normalizeRepo(map[string]any{"path": "/projects/my-repo"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "my-repo" {
		t.Errorf("expected name 'my-repo', got %s", result.Name)
	}
}

func TestNormalizeRepo_UnknownMemoryManagerFallsBack(t *testing.T) {
	dir := t.TempDir()
	result := normalizeRepo(map[string]any{"path": dir, "memoryManagerType": "unknown"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.MemoryManagerType != MemoryManagerBeads {
		t.Errorf("expected beads fallback for unknown type, got %s", result.MemoryManagerType)
	}
}

func TestSaveRegistry_CreatesDirAndFile(t *testing.T) {
	registryDir := t.TempDir()
	registryFile := filepath.Join(registryDir, "registry.json")

	origDir := configDir
	origPath := registryFilePath
	configDir = func() string { return registryDir }
	registryFilePath = func() string { return registryFile }
	t.Cleanup(func() {
		configDir = origDir
		registryFilePath = origPath
	})

	reg := Registry{Repos: []RegistryRepo{{Path: "/tmp/test", Name: "test", MemoryManagerType: MemoryManagerBeads}}}
	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	if _, err := os.Stat(registryFile); os.IsNotExist(err) {
		t.Error("registry file was not created")
	}

	info, _ := os.Stat(registryFile)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestAddRepo_Success(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _ = setupTestRegistry(t)

	repo, err := AddRepo(dir)
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if repo.Path != dir {
		t.Errorf("expected path %s, got %s", dir, repo.Path)
	}
	if repo.MemoryManagerType != MemoryManagerBeads {
		t.Errorf("expected beads, got %s", repo.MemoryManagerType)
	}
	if repo.Name == "" {
		t.Error("expected non-empty name")
	}
}

func TestAddRepo_DuplicateRejects(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _ = setupTestRegistry(t)

	_, err := AddRepo(dir)
	if err != nil {
		t.Fatalf("first AddRepo: %v", err)
	}

	_, err = AddRepo(dir)
	if err == nil {
		t.Error("expected error for duplicate repo")
	}
}

func TestRemoveRepo(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _ = AddRepo(dir)
	if err := RemoveRepo(dir); err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}

	data, _ := os.ReadFile(registryFile)
	var reg Registry
	_ = json.Unmarshal(data, &reg)
	for _, r := range reg.Repos {
		if r.Path == dir {
			t.Error("repo still present after removal")
		}
	}
}

func TestListRepos(t *testing.T) {
	_, _ = setupTestRegistry(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := AddRepo(dir)
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	repos, err := ListRepos()
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}
}

func TestInspectMissingRepoMemoryManagerTypes_ReportsMissing(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{
		"repos": []any{
			map[string]any{"path": "/repo1", "name": "repo1"},
			map[string]any{"path": "/repo2", "name": "repo2", "memoryManagerType": "knots"},
		},
	})

	result := InspectMissingRepoMemoryManagerTypes()
	if result.FileMissing {
		t.Error("expected file to exist")
	}
	if len(result.MissingRepoPaths) != 1 {
		t.Errorf("expected 1 missing repo path, got %d", len(result.MissingRepoPaths))
	}
	if len(result.MissingRepoPaths) > 0 && result.MissingRepoPaths[0] != "/repo1" {
		t.Errorf("expected /repo1, got %s", result.MissingRepoPaths[0])
	}
}

func TestInspectMissingRepoMemoryManagerTypes_MissingFile(t *testing.T) {
	_, _ = setupTestRegistry(t)
	result := InspectMissingRepoMemoryManagerTypes()
	if !result.FileMissing {
		t.Error("expected fileMissing to be true")
	}
}

func TestBackfillMissingRepoMemoryManagerTypes_WritesMissing(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{
		"repos": []any{
			map[string]any{"path": "/repo1", "name": "repo1"},
			map[string]any{"path": "/repo2", "name": "repo2", "memoryManagerType": "knots"},
		},
	})

	result := BackfillMissingRepoMemoryManagerTypes()
	if !result.Changed {
		t.Error("expected changed=true")
	}
	if len(result.MigratedRepoPaths) != 1 {
		t.Errorf("expected 1 migrated path, got %d", len(result.MigratedRepoPaths))
	}
}

func TestBackfillMissingRepoMemoryManagerTypes_NoChange(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{
		"repos": []any{
			map[string]any{"path": "/repo1", "name": "repo1", "memoryManagerType": "beads"},
		},
	})

	result := BackfillMissingRepoMemoryManagerTypes()
	if result.Changed {
		t.Error("expected no change when all repos have memoryManagerType")
	}
}

func TestBackfillMissingRepoMemoryManagerTypes_MissingFile(t *testing.T) {
	_, _ = setupTestRegistry(t)
	result := BackfillMissingRepoMemoryManagerTypes()
	if !result.FileMissing {
		t.Error("expected fileMissing=true")
	}
}

func TestInspectRegistryPermissions_CorrectPermissions(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{"repos": []any{}})

	result := InspectRegistryPermissions()
	if result.FileMissing {
		t.Error("expected file to exist")
	}
	if result.NeedsFix {
		t.Errorf("expected no fix needed, got needsFix=true, mode=%o", result.ActualMode)
	}
}

func TestEnsureRegistryPermissions_FixesLoosePermissions(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{"repos": []any{}})
	_ = os.Chmod(registryFile, 0o644)

	result := EnsureRegistryPermissions()
	if !result.Changed {
		t.Error("expected permissions to be fixed")
	}

	info, _ := os.Stat(registryFile)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestEnsureRegistryPermissions_NoFixNeeded(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{"repos": []any{}})

	result := EnsureRegistryPermissions()
	if result.Changed {
		t.Error("expected no change when permissions are correct")
	}
}

func TestUpdateRegisteredRepoMemoryManagerType_ChangesType(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{
		"repos": []any{
			map[string]any{"path": "/repo1", "name": "repo1", "memoryManagerType": "beads"},
		},
	})

	result := UpdateRegisteredRepoMemoryManagerType("/repo1", MemoryManagerKnots)
	if !result.Changed {
		t.Error("expected changed=true")
	}
	if !result.RepoFound {
		t.Error("expected repoFound=true")
	}
	if result.PreviousMemoryManagerType != MemoryManagerBeads {
		t.Errorf("expected previous beads, got %s", result.PreviousMemoryManagerType)
	}
}

func TestUpdateRegisteredRepoMemoryManagerType_NoChange(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{
		"repos": []any{
			map[string]any{"path": "/repo1", "name": "repo1", "memoryManagerType": "beads"},
		},
	})

	result := UpdateRegisteredRepoMemoryManagerType("/repo1", MemoryManagerBeads)
	if result.Changed {
		t.Error("expected no change when type is the same")
	}
}

func TestUpdateRegisteredRepoMemoryManagerType_RepoNotFound(t *testing.T) {
	_, registryFile := setupTestRegistry(t)
	writeTestRegistry(t, registryFile, map[string]any{
		"repos": []any{
			map[string]any{"path": "/repo1", "name": "repo1", "memoryManagerType": "beads"},
		},
	})

	result := UpdateRegisteredRepoMemoryManagerType("/nonexistent", MemoryManagerKnots)
	if result.RepoFound {
		t.Error("expected repoFound=false")
	}
}

func TestNormalizeRegistry_SkipsInvalidEntries(t *testing.T) {
	result := normalizeRegistry(map[string]any{
		"repos": []any{
			map[string]any{"path": "/valid", "name": "valid"},
			map[string]any{"name": "missing-path"},
			"not-a-map",
		},
	})
	if len(result.Repos) != 1 {
		t.Errorf("expected 1 valid repo, got %d", len(result.Repos))
	}
	if len(result.Repos) > 0 && result.Repos[0].Path != "/valid" {
		t.Errorf("expected /valid, got %s", result.Repos[0].Path)
	}
}

func TestNormalizeRegistry_NilRepos(t *testing.T) {
	result := normalizeRegistry(map[string]any{"other": "data"})
	if result.Repos != nil {
		t.Errorf("expected nil repos, got %v", result.Repos)
	}
}

func TestExtractBaseName(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected string
	}{
		{"/home/user/my-repo", "my-repo"},
		{"/home/user/my-repo/", "my-repo"},
		{"simple", "simple"},
		{"", ""},
	} {
		result := extractBaseName(tc.input)
		if result != tc.expected {
			t.Errorf("extractBaseName(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// A declared memoryManager skips detection entirely, so nothing ever compares
// the declaration against what is on disk. A repository declaring a tracker it
// was never initialized for built a backend happily and failed on its first
// call instead - once per sweep tick, forever, with `kernl doctor` reporting
// the configuration healthy throughout.
func TestTrackerStorePath_RefusesARepositoryThatHasNoStore(t *testing.T) {
	dir := t.TempDir()

	_, err := TrackerStorePath(MemoryManagerBeads, dir)
	if err == nil {
		t.Fatal("a repository with no .beads/embeddeddolt has no bd store and must not resolve one")
	}
	if !strings.Contains(err.Error(), "embeddeddolt") {
		t.Errorf("the error must name the store that is missing, got %q", err)
	}
	if !strings.Contains(err.Error(), "Fix:") {
		t.Errorf("the error must carry the fix, got %q", err)
	}
}

func TestTrackerStorePath_FindsEachTrackersStore(t *testing.T) {
	t.Run("bd stores under embeddeddolt", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".beads", "embeddeddolt"), 0o755); err != nil {
			t.Fatal(err)
		}
		store, err := TrackerStorePath(MemoryManagerBeads, dir)
		if err != nil {
			t.Fatalf("an initialized bd repository must resolve its store: %v", err)
		}
		if store != filepath.Join(dir, ".beads", "embeddeddolt") {
			t.Errorf("unexpected store path %q", store)
		}
	})

	// br names its database freely, so the store is whichever *.db sits in
	// .beads/ - the same scan the adapter does to build --db. Insisting on the
	// fixed name detection matches would report a working repository broken.
	t.Run("br stores a database whose name it chooses", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".beads", "renamed.db"), []byte("sqlite"), 0o644); err != nil {
			t.Fatal(err)
		}
		store, err := TrackerStorePath(MemoryManagerBeadsRust, dir)
		if err != nil {
			t.Fatalf("an initialized br repository must resolve its store: %v", err)
		}
		if store != filepath.Join(dir, ".beads", "renamed.db") {
			t.Errorf("unexpected store path %q", store)
		}
	})

	t.Run("knots stores in a marker directory of its own", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".knots"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := TrackerStorePath(MemoryManagerKnots, dir); err != nil {
			t.Fatalf("an initialized knots repository must resolve its store: %v", err)
		}
	})
}

func TestTrackerStorePath_RefusesATrackerItDoesNotKnow(t *testing.T) {
	if _, err := TrackerStorePath(MemoryManagerType("beadz"), t.TempDir()); err == nil {
		t.Fatal("a tracker with no registered store layout must not resolve a path")
	}
}
