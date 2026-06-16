package dispatch

import "testing"

func TestNormalizeWaveSlugCandidate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  Streep Montage  ", "streep-montage"},
		{"Heat___Take__2", "heat-take-2"},
		{"Hello World", "hello-world"},
		{"foo_bar", "foo-bar"},
		{"  spaces  ", "spaces"},
		{"UPPER", "upper"},
		{"--foo--", "foo"},
		{"", ""},
		{"a!!!b", "a-b"},
		{"123", "123"},
	}

	for _, tt := range tests {
		got := NormalizeWaveSlugCandidate(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeWaveSlugCandidate(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsWaveLabel(t *testing.T) {
	if !IsWaveLabel(OrchestrationWaveLabel) {
		t.Error("IsWaveLabel should match exact wave label")
	}
	if !IsWaveLabel(OrchestrationWaveLabelPrefix + "my-slug") {
		t.Error("IsWaveLabel should match slug label")
	}
	if IsWaveLabel("stage:verification") {
		t.Error("IsWaveLabel should not match unrelated label")
	}
}

func TestIsInternalLabel(t *testing.T) {
	if !IsInternalLabel(OrchestrationWaveLabel) {
		t.Error("IsInternalLabel should match wave labels")
	}
	if !IsInternalLabel("stage:verification") {
		t.Error("IsInternalLabel should match stage labels")
	}
	if !IsInternalLabel("stage:retry") {
		t.Error("IsInternalLabel should match stage:retry")
	}
	if IsInternalLabel("frontend") {
		t.Error("IsInternalLabel should not match user labels")
	}
}

func TestIsReadOnlyLabel(t *testing.T) {
	if !IsReadOnlyLabel("attempts:1") {
		t.Error("IsReadOnlyLabel should match attempts:1")
	}
	if !IsReadOnlyLabel("attempts:99") {
		t.Error("IsReadOnlyLabel should match attempts:99")
	}
	if IsReadOnlyLabel("stage:retry") {
		t.Error("IsReadOnlyLabel should not match stage labels")
	}
	if IsReadOnlyLabel("frontend") {
		t.Error("IsReadOnlyLabel should not match user labels")
	}
}

func TestIsWaveSlugLabel(t *testing.T) {
	if !IsWaveSlugLabel(OrchestrationWaveLabelPrefix + "slug") {
		t.Error("IsWaveSlugLabel should match slug labels")
	}
	if IsWaveSlugLabel(OrchestrationWaveLabel) {
		t.Error("IsWaveSlugLabel should not match bare wave label")
	}
}

func TestGetWaveSlugLabels(t *testing.T) {
	labels := []string{
		"foo",
		OrchestrationWaveLabelPrefix + "a",
		OrchestrationWaveLabel,
		OrchestrationWaveLabelPrefix + "b",
	}
	result := GetWaveSlugLabels(labels)
	if len(result) != 2 {
		t.Fatalf("expected 2 wave slug labels, got %d", len(result))
	}
	if result[0] != OrchestrationWaveLabelPrefix+"a" {
		t.Errorf("first should be 'a', got %s", result[0])
	}
	if result[1] != OrchestrationWaveLabelPrefix+"b" {
		t.Errorf("second should be 'b', got %s", result[1])
	}

	empty := GetWaveSlugLabels([]string{"foo", "bar"})
	if len(empty) != 0 {
		t.Error("expected no wave slug labels from non-wave labels")
	}
}

func TestExtractWaveSlug(t *testing.T) {
	labels := []string{"foo:bar", OrchestrationWaveLabel, OrchestrationWaveLabelPrefix + "pacino-dolly"}
	got := ExtractWaveSlug(labels)
	if got != "pacino-dolly" {
		t.Errorf("ExtractWaveSlug: got %q, want %q", got, "pacino-dolly")
	}

	if ExtractWaveSlug([]string{}) != "" {
		t.Error("ExtractWaveSlug should return empty string for empty labels")
	}

	if ExtractWaveSlug([]string{OrchestrationWaveLabelPrefix}) != "" {
		t.Error("ExtractWaveSlug should return empty string for empty slug label")
	}

	if ExtractWaveSlug([]string{OrchestrationWaveLabelPrefix + "   "}) != "" {
		t.Error("ExtractWaveSlug should return empty string for whitespace slug label")
	}

	multi := []string{
		OrchestrationWaveLabelPrefix + "first",
		OrchestrationWaveLabelPrefix + "second",
	}
	if ExtractWaveSlug(multi) != "first" {
		t.Error("ExtractWaveSlug should return first valid slug when multiple exist")
	}
}

func TestIsLegacyNumericWaveSlug(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1", true},
		{"022", true},
		{"0", true},
		{"", false},
		{"heat-1", false},
		{"abc", false},
	}

	for _, tt := range tests {
		got := IsLegacyNumericWaveSlug(tt.input)
		if got != tt.want {
			t.Errorf("IsLegacyNumericWaveSlug(%q): got %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBuildWaveSlugLabel(t *testing.T) {
	got := BuildWaveSlugLabel("streep-montage")
	want := OrchestrationWaveLabelPrefix + "streep-montage"
	if got != want {
		t.Errorf("BuildWaveSlugLabel: got %q, want %q", got, want)
	}

	got2 := BuildWaveSlugLabel("My Slug")
	want2 := OrchestrationWaveLabelPrefix + "my-slug"
	if got2 != want2 {
		t.Errorf("BuildWaveSlugLabel normalizes: got %q, want %q", got2, want2)
	}
}

func TestAllocateWaveSlug(t *testing.T) {
	used := map[string]bool{"streep-montage": true}
	preferred := AllocateWaveSlug(used, "Pacino Dolly")
	if preferred != "pacino-dolly" {
		t.Errorf("AllocateWaveSlug preferred: got %q, want %q", preferred, "pacino-dolly")
	}
	if !used["pacino-dolly"] {
		t.Error("AllocateWaveSlug should add preferred to usedSet")
	}

	gen1 := AllocateWaveSlug(used, "")
	gen2 := AllocateWaveSlug(used, "")
	if gen1 == gen2 {
		t.Error("AllocateWaveSlug should generate unique slugs")
	}
	if !used[gen1] || !used[gen2] {
		t.Error("AllocateWaveSlug should add generated slugs to usedSet")
	}

	taken := AllocateWaveSlug(map[string]bool{"my-slug": true}, "My Slug")
	if taken == "my-slug" {
		t.Error("AllocateWaveSlug should not return already-used preferred slug")
	}

	empty := AllocateWaveSlug(map[string]bool{}, "")
	if empty == "" {
		t.Error("AllocateWaveSlug should generate slug when no preferred given")
	}

	ws := AllocateWaveSlug(map[string]bool{}, "   ")
	if ws == "" {
		t.Error("AllocateWaveSlug should generate slug for whitespace-only preferred")
	}

	set := map[string]bool{}
	slugs := make(map[string]bool)
	for i := 0; i < 10; i++ {
		s := AllocateWaveSlug(set, "")
		if slugs[s] {
			t.Errorf("AllocateWaveSlug generated duplicate: %q", s)
		}
		slugs[s] = true
	}
}

func TestBuildWaveTitle(t *testing.T) {
	if got := BuildWaveTitle("streep-montage", "Backend unblockers"); got != "Scene streep-montage: Backend unblockers" {
		t.Errorf("BuildWaveTitle: got %q, want %q", got, "Scene streep-montage: Backend unblockers")
	}
	if got := BuildWaveTitle("my-slug", ""); got != "Scene my-slug" {
		t.Errorf("BuildWaveTitle empty name: got %q, want %q", got, "Scene my-slug")
	}
	if got := BuildWaveTitle("my-slug", "   "); got != "Scene my-slug" {
		t.Errorf("BuildWaveTitle whitespace name: got %q, want %q", got, "Scene my-slug")
	}
	if got := BuildWaveTitle("s", "  Hello  "); got != "Scene s: Hello" {
		t.Errorf("BuildWaveTitle trimmed name: got %q, want %q", got, "Scene s: Hello")
	}
}

func TestRewriteWaveTitleSlug(t *testing.T) {
	if got := RewriteWaveTitleSlug("Wave 1: Backend unblockers", "streep-montage"); got != "Scene streep-montage: Backend unblockers" {
		t.Errorf("RewriteWaveTitleSlug wave: got %q, want %q", got, "Scene streep-montage: Backend unblockers")
	}
	if got := RewriteWaveTitleSlug("", "new"); got != "Scene new" {
		t.Errorf("RewriteWaveTitleSlug empty: got %q, want %q", got, "Scene new")
	}
	if got := RewriteWaveTitleSlug("  ", "new"); got != "Scene new" {
		t.Errorf("RewriteWaveTitleSlug whitespace: got %q, want %q", got, "Scene new")
	}
	if got := RewriteWaveTitleSlug("Scene old: Backend", "new"); got != "Scene new: Backend" {
		t.Errorf("RewriteWaveTitleSlug scene: got %q, want %q", got, "Scene new: Backend")
	}
	if got := RewriteWaveTitleSlug("Just a title", "slug"); got != "Scene slug: Just a title" {
		t.Errorf("RewriteWaveTitleSlug no prefix: got %q, want %q", got, "Scene slug: Just a title")
	}
	if got := RewriteWaveTitleSlug("WAVE 1: test", "s"); got != "Scene s: test" {
		t.Errorf("RewriteWaveTitleSlug case insensitive wave: got %q, want %q", got, "Scene s: test")
	}
	if got := RewriteWaveTitleSlug("SCENE old: test", "s"); got != "Scene s: test" {
		t.Errorf("RewriteWaveTitleSlug case insensitive scene: got %q, want %q", got, "Scene s: test")
	}
}
