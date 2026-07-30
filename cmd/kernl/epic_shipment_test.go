package main

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func appWithEpicDescription(description string) (*app.App, *epicRunTestBackend) {
	be := &epicRunTestBackend{
		beads: []backend.Bead{
			{ID: "e", Type: "epic", Title: "test epic", State: "awaiting_pr_review", Description: description},
		},
	}
	return &app.App{Backend: be}, be
}

// The drive loop advances the bead the moment the exit gate passes, so a pull
// request that turns out to be outside the allow-list is discovered when the
// tracker already reads "awaiting_pr_review". Leaving it there would have the
// tracker report a successful run - and the tracker is what the next session
// reads, not this process's exit code.
func TestVerifyPublishedPullRequest_BlocksTheEpicWhenThePRIsNotAllowed(t *testing.T) {
	a, be := appWithEpicDescription("pr_url: https://github.com/someone-else/archeion/pull/1")
	plan := shipmentPlan{Allowed: []string{"git@github.com:gabrielassisxyz/archeion.git"}}

	err := verifyPublishedPullRequest(a, "e", "/repo", plan)
	if err == nil {
		t.Fatal("expected a refusal for a pull request outside the allow-list")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error lacks the greppable marker: %v", err)
	}
	got, _ := be.Get("e", "/repo")
	if got.State != string(workflow.StatusBlocked) {
		t.Errorf("epic state = %q, want blocked - a run that published outside the allow-list must not read as successful", got.State)
	}
}

func TestVerifyPublishedPullRequest_AllowsAndLeavesStateAlone(t *testing.T) {
	a, be := appWithEpicDescription("pr_url: https://github.com/gabrielassisxyz/archeion/pull/40")
	plan := shipmentPlan{Allowed: []string{"git@github.com:gabrielassisxyz/archeion.git"}}

	if err := verifyPublishedPullRequest(a, "e", "/repo", plan); err != nil {
		t.Fatalf("allowed pull request rejected: %v", err)
	}
	got, _ := be.Get("e", "/repo")
	if got.State != "awaiting_pr_review" {
		t.Errorf("epic state = %q, want awaiting_pr_review untouched", got.State)
	}
}

// A dry run never reaches shipment, so there is nothing to verify and no state
// to touch.
func TestVerifyPublishedPullRequest_DryRunSkips(t *testing.T) {
	a, _ := appWithEpicDescription("pr_url: https://github.com/someone-else/archeion/pull/1")
	plan := shipmentPlan{Allowed: nil, DryRun: true}

	if err := verifyPublishedPullRequest(a, "e", "/repo", plan); err != nil {
		t.Fatalf("dry run must not verify anything: %v", err)
	}
}

// Shipment that never got as far as a pull request is the stage's own failure
// to report, not a containment breach.
func TestVerifyPublishedPullRequest_NoPRURLIsNotAViolation(t *testing.T) {
	a, _ := appWithEpicDescription("merge_outcome: push_failed")
	plan := shipmentPlan{Allowed: []string{"git@github.com:gabrielassisxyz/archeion.git"}}

	if err := verifyPublishedPullRequest(a, "e", "/repo", plan); err != nil {
		t.Fatalf("absent pr_url must not be treated as a violation: %v", err)
	}
}
