package merge_test

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/merge"
)

func TestParseOutcome_Valid(t *testing.T) {
	cases := map[string]merge.Outcome{
		"success":           merge.OutcomeSuccess,
		"merge_conflict":    merge.OutcomeMergeConflict,
		"push_failed":       merge.OutcomePushFailed,
		"pr_create_failed":  merge.OutcomePRCreateFailed,
		"pr_already_exists": merge.OutcomePRAlreadyExists,
	}
	for s, want := range cases {
		t.Run(s, func(t *testing.T) {
			got, err := merge.ParseOutcome(s)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}

func TestParseOutcome_Invalid(t *testing.T) {
	if _, err := merge.ParseOutcome("nope"); err == nil {
		t.Fatal("expected error for unknown outcome")
	}
	if _, err := merge.ParseOutcome(""); err == nil {
		t.Fatal("expected error for empty outcome")
	}
}
