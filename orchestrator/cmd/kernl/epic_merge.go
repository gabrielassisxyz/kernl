package main

import (
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/merge"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

var mergeFn func(a *app.App, args []string) error = runEpicMergeWithApp

func runEpicMerge(a *app.App, args []string) error {
	return mergeFn(a, args)
}

func runEpicMergeWithApp(a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic merge requires an epic ID — run: kernl epic merge <epic-id>")
	}
	if len(a.Config.Registry.Repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered — Fix: add a repo to registry.repos in kernl.yaml")
	}
	if a.Merger == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no merge manager configured — Fix: wire a merge.MergeDispatchPort to App.Merger")
	}

	epicID := args[0]
	repoPath := a.Config.Registry.Repos[0].Path

	ep, err := epic.LoadEpic(a.Backend, epicID, repoPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: loading epic %s: %w", epicID, err)
	}

	epicBead, err := a.Backend.Get(epicID, repoPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: fetching epic bead %s: %w", epicID, err)
	}
	if epicBead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found — check the epic ID and repo path", epicID)
	}

	if err := validateEpicForMerge(epicBead, ep.Children); err != nil {
		return err
	}

	desc := epicBead.Description
	desc = workflow.RemoveMetadataField(desc, "merge_conflict_at")
	desc = workflow.RemoveMetadataField(desc, "merge_outcome")

	if err := a.Backend.Update(epicID, backend.UpdateBeadInput{
		State:       string(workflow.StatusInProgress),
		Description: desc,
	}, repoPath); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: updating epic %s to in_progress: %w", epicID, err)
	}

	if err := a.Merger.DispatchMerger(epicID); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: dispatching merger for epic %s: %w", epicID, err)
	}

	return nil
}

func validateEpicForMerge(epicBead *backend.Bead, children []backend.Bead) error {
	if epicBead.State != string(workflow.StatusBlocked) {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s is not blocked (current state: %s) — merge requires a blocked epic — Fix: only blocked epics can be manually merged", epicBead.ID, epicBead.State)
	}

	mergeConflictAt := workflow.GetMergeConflictAt(epicBead.Description)
	mergeOutcome := workflow.GetMergeOutcome(epicBead.Description)

	hasRecoverySignal := mergeConflictAt != ""
	if !hasRecoverySignal {
		o, err := merge.ParseOutcome(mergeOutcome)
		if err == nil && (o == merge.OutcomePushFailed || o == merge.OutcomePRCreateFailed) {
			hasRecoverySignal = true
		}
	}

	if !hasRecoverySignal {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s is blocked but has no recovery signal — missing merge_conflict_at or recoverable merge_outcome (push_failed / pr_create_failed) — Fix: set merge_conflict_at or merge_outcome in the epic description first", epicBead.ID)
	}

	for _, child := range children {
		if child.State == string(workflow.StatusInProgress) {
			return fmt.Errorf("KERNL DISPATCH FAILURE: child bead %s of epic %s is still in_progress — all children must be in awaiting_integration or closed before merging — Fix: wait for all child beads to finish", child.ID, epicBead.ID)
		}
		if child.State != string(workflow.StatusAwaitingIntegration) && child.State != string(workflow.StatusClosed) {
			return fmt.Errorf("KERNL DISPATCH FAILURE: child bead %s of epic %s is in state %s — all children must be in awaiting_integration or closed before merging — Fix: ensure all child beads have reached awaiting_integration or closed", child.ID, epicBead.ID, child.State)
		}
	}

	return nil
}
