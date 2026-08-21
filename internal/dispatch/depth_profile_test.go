package dispatch

import (
	"slices"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// allDepths is the exhaustive list of Depth values this package defines.
// Kept next to the test that walks it, so a future depth added to depth.go
// without a matching entry here is a one-line diff to notice in review -
// and, more importantly, ProfileForDepth's own panicking default (see
// depth_profile.go) is what actually catches the omission at runtime.
var allDepths = []Depth{DepthGate, DepthFullPipeline, DepthShortFlow}

// TestProfileForDepth_HandlesEveryDepth asserts ProfileForDepth's switch has
// an explicit case for every depth this package defines today. Because that
// switch panics on anything else, this test passing is what proves the
// three known values are handled - and a fourth depth added later without a
// matching case would panic here instead of silently falling through.
func TestProfileForDepth_HandlesEveryDepth(t *testing.T) {
	for _, d := range allDepths {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ProfileForDepth(%q) panicked: %v - every Depth value must be handled explicitly", d, r)
				}
			}()
			ProfileForDepth(d)
		}()
	}
}

// TestProfileForDepth_GateHasNoProfile pins DepthGate's mapping as an
// explicit "no profile", not an omission: a gated bead is never dispatched,
// so it has nothing to run under.
func TestProfileForDepth_GateHasNoProfile(t *testing.T) {
	profileID, ok := ProfileForDepth(DepthGate)
	if ok {
		t.Fatalf("ProfileForDepth(DepthGate) = (%q, true), want ok=false - a gated bead must map to no profile", profileID)
	}
}

// TestProfileForDepth_FullPipelineAndShortFlowUseBuiltinProfiles asserts the
// two dispatched depths each map to a profile ID that actually exists among
// the built-in profiles, by resolving it through the same
// BuiltinProfileDescriptor lookup the dispatcher itself uses.
func TestProfileForDepth_FullPipelineAndShortFlowUseBuiltinProfiles(t *testing.T) {
	for _, d := range []Depth{DepthFullPipeline, DepthShortFlow} {
		profileID, ok := ProfileForDepth(d)
		if !ok {
			t.Fatalf("ProfileForDepth(%q) reported ok=false, want a built-in profile", d)
		}
		desc := backend.BuiltinProfileDescriptor(profileID)
		if desc.ID != profileID {
			t.Errorf("ProfileForDepth(%q) = %q, which BuiltinProfileDescriptor does not recognize (fell back to %q)", d, profileID, desc.ID)
		}
	}
}

// TestProfileForDepth_FullPipelineAndShortFlowDifferInPlanning asserts the
// two mapped profiles differ in exactly the axis this whole mapping exists
// to select: whether a planning stage runs. It reads that off the actual
// descriptor's own state list - which drops "planning" and "plan_review"
// precisely when a profile's PlanningMode is "skipped" (see buildStates in
// internal/backend/state_machine.go) - rather than hardcoding either
// profile's name or its PlanningMode value a second time here.
func TestProfileForDepth_FullPipelineAndShortFlowDifferInPlanning(t *testing.T) {
	fullID, _ := ProfileForDepth(DepthFullPipeline)
	shortID, _ := ProfileForDepth(DepthShortFlow)

	fullDesc := backend.BuiltinProfileDescriptor(fullID)
	shortDesc := backend.BuiltinProfileDescriptor(shortID)

	if !slices.Contains(fullDesc.States, "planning") {
		t.Errorf("full_pipeline profile %q must run a planning stage, states = %v", fullID, fullDesc.States)
	}
	if slices.Contains(shortDesc.States, "planning") {
		t.Errorf("short_flow profile %q must skip the planning stage, states = %v", shortID, shortDesc.States)
	}
}
