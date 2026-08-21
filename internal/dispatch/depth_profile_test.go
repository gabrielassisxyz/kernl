package dispatch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// depthConstants discovers every Depth constant declared in depth.go by
// parsing its source with go/ast. Keeping this in the test rather than as a
// hand-maintained slice means adding a fourth Depth constant without also
// adding a matching case to ProfileForDepth fails the test suite: the new
// value is discovered here and exercised, and ProfileForDepth panics on it.
func depthConstants(t *testing.T) []Depth {
	t.Helper()
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "depth.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing depth.go: %v", err)
	}

	var values []Depth
	for _, decl := range node.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if vs.Type == nil {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Depth" {
				continue
			}
			for _, val := range vs.Values {
				lit, ok := val.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("Depth constant in depth.go is not a string literal: %v", val)
				}
				// lit.Value includes the surrounding quotes.
				values = append(values, Depth(lit.Value[1:len(lit.Value)-1]))
			}
		}
	}
	if len(values) == 0 {
		t.Fatal("found no Depth constants in depth.go - the AST helper is broken")
	}
	return values
}

// TestProfileForDepth_HandlesEveryDepth asserts ProfileForDepth's switch has
// an explicit case for every Depth constant this package declares. Because
// that switch panics on anything else, a future fourth depth added to depth.go
// without a matching case fails here instead of silently falling through.
func TestProfileForDepth_HandlesEveryDepth(t *testing.T) {
	for _, d := range depthConstants(t) {
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

// TestProfileForDepth_FullPipelineAndShortFlowDifferOnlyInPlanning asserts the
// two mapped profiles differ in exactly the axis this whole mapping exists
// to select: whether a planning stage runs. It reads that off the actual
// descriptor's own state list - which drops "planning" and "plan_review"
// precisely when a profile's PlanningMode is "skipped" (see buildStates in
// internal/backend/state_machine.go) - rather than hardcoding either
// profile's name or its PlanningMode value a second time here. It also
// asserts the descriptors are otherwise identical, so the mapping does not
// silently change output, owners, exit gates or review semantics.
func TestProfileForDepth_FullPipelineAndShortFlowDifferOnlyInPlanning(t *testing.T) {
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

	if fullDesc.Mode != shortDesc.Mode {
		t.Errorf("mapped profiles must have the same Mode, got %q and %q", fullDesc.Mode, shortDesc.Mode)
	}
	// InitialState and States are allowed to differ because both are
	// derived from PlanningMode. The whole point of the mapping is that
	// the two profiles differ only on that planning axis, not on ownership,
	// exit gates, terminal states or mode.
	if fullDesc.InitialState != "ready_for_planning" {
		t.Errorf("full_pipeline profile %q must start at ready_for_planning, got %q", fullID, fullDesc.InitialState)
	}
	if shortDesc.InitialState != "ready_for_implementation" {
		t.Errorf("short_flow profile %q must start at ready_for_implementation, got %q", shortID, shortDesc.InitialState)
	}
	if !slices.Equal(fullDesc.TerminalStates, shortDesc.TerminalStates) {
		t.Errorf("mapped profiles must have the same TerminalStates, got %v and %v", fullDesc.TerminalStates, shortDesc.TerminalStates)
	}
	if len(fullDesc.ExitGates) != 0 || len(shortDesc.ExitGates) != 0 {
		t.Errorf("mapped profiles must carry no exit gates, got %v and %v", fullDesc.ExitGates, shortDesc.ExitGates)
	}
	if !mapsEqual(fullDesc.Owners, shortDesc.Owners) {
		t.Errorf("mapped profiles must have the same owners, got %v and %v", fullDesc.Owners, shortDesc.Owners)
	}
}

func mapsEqual(a, b map[string]backend.ActionOwnerKind) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
