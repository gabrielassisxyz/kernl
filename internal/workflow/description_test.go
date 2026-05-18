package workflow_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func TestGetMetadataField_Variants(t *testing.T) {
	cases := []struct {
		name string
		desc string
		key  string
		want string
	}{
		{"basic", "pr_url: https://x/pr/1\n", "pr_url", "https://x/pr/1"},
		{"case_insens_key", "PR_URL: x\n", "pr_url", "x"},
		{"colon_in_value", "url: https://example.com:443/x\n", "url", "https://example.com:443/x"},
		{"empty_desc", "", "pr_url", ""},
		{"absent_key", "other: y\n", "pr_url", ""},
		{"multiline_doc", "line one\npr_url: x\nline three\n", "pr_url", "x"},
		{"bom_prefix", "\ufeffpr_url: x\n", "pr_url", "x"},
		{"trailing_spaces", "pr_url:    x   \n", "pr_url", "x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := workflow.GetMetadataField(c.desc, c.key); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestAddMetadataField_Insert(t *testing.T) {
	out := workflow.AddMetadataField("body text\n", "pr_url", "https://x/pr/1")
	if !strings.Contains(out, "pr_url: https://x/pr/1") {
		t.Fatalf("missing inserted field, got: %q", out)
	}
	if !strings.Contains(out, "body text") {
		t.Fatalf("original body lost: %q", out)
	}
}

func TestAddMetadataField_Update(t *testing.T) {
	in := "body\npr_url: old\nfooter\n"
	out := workflow.AddMetadataField(in, "pr_url", "new")
	if strings.Contains(out, "old") {
		t.Fatalf("old value not removed: %q", out)
	}
	if !strings.Contains(out, "pr_url: new") {
		t.Fatalf("new value missing: %q", out)
	}
	if !strings.Contains(out, "footer") {
		t.Fatalf("unrelated line lost: %q", out)
	}
}

func TestTypedHelpers_Roundtrip(t *testing.T) {
	desc := ""
	desc = workflow.SetPRURL(desc, "https://x/pr/42")
	desc = workflow.SetEpicBranch(desc, "feat/kernl-abc")
	desc = workflow.SetMergeOutcome(desc, "success")
	if got := workflow.GetPRURL(desc); got != "https://x/pr/42" {
		t.Fatalf("pr_url roundtrip: %q", got)
	}
	if got := workflow.GetEpicBranch(desc); got != "feat/kernl-abc" {
		t.Fatalf("epic_branch roundtrip: %q", got)
	}
	if got := workflow.GetMergeOutcome(desc); got != "success" {
		t.Fatalf("merge_outcome roundtrip: %q", got)
	}
}
