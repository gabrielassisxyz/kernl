package prompt_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func TestMergerPrompt_Golden(t *testing.T) {
	cases := []struct {
		name  string
		input prompt.MergerInput
	}{
		{"N1", prompt.MergerInput{EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
			Children: []prompt.Child{{ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1"}}}},
		{"N3", prompt.MergerInput{EpicID: "e1", EpicTitle: "Test epic", EpicBranch: "feat/e1", BaseBranch: "master",
			Children: []prompt.Child{
				{ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1"},
				{ID: "c2", Branch: "feat/c2", WorktreePath: "/tmp/c2"},
				{ID: "c3", Branch: "feat/c3", WorktreePath: "/tmp/c3"},
			}}},
		{"N10", prompt.MergerInput{EpicID: "e1", EpicTitle: "Big epic", EpicBranch: "feat/e1", BaseBranch: "master",
			Children: func() []prompt.Child {
				cs := make([]prompt.Child, 10)
				for i := range cs {
					cs[i] = prompt.Child{
						ID:           fmt.Sprintf("c%d", i+1),
						Branch:       fmt.Sprintf("feat/c%d", i+1),
						WorktreePath: fmt.Sprintf("/tmp/c%d", i+1),
					}
				}
				return cs
			}()}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := prompt.RenderMerger(c.input)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "merger_prompt_"+c.name+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				_ = os.WriteFile(path, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != got {
				t.Fatalf("golden mismatch — re-run with UPDATE_GOLDEN=1 if intentional\n--- want ---\n%s\n--- got ---\n%s", string(want), got)
			}
		})
	}
}

func TestMergerPrompt_ContainsAllOutcomes(t *testing.T) {
	out, err := prompt.RenderMerger(prompt.MergerInput{
		EpicID: "e1", EpicTitle: "x", EpicBranch: "feat/e1", BaseBranch: "master",
		Children: []prompt.Child{{ID: "c1", Branch: "feat/c1", WorktreePath: "/tmp/c1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Must enumerate the literal outcome list (D6=A).
	for _, want := range []string{"success", "merge_conflict", "push_failed", "pr_create_failed", "pr_already_exists"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing outcome %q in rendered prompt", want)
		}
	}
}
