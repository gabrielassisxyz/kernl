package dispatch

import "fmt"

// ProfileForDepth maps a classified Depth onto the built-in workflow
// profile the dispatcher runs a bead under, so ClassifyDepth's answer
// selects something instead of being advice with no consumer.
//
// DepthGate maps to no profile at all (ok is false): a gated bead is never
// dispatched, so there is nothing for it to run under. The other two map to
// profiles that differ in exactly one axis - whether planning runs - and
// are otherwise identical (same Output, same implementation review mode),
// so the mapping isolates the depth decision instead of also changing
// unrelated behaviour.
//
// The switch has no default case that returns a zero value: an unhandled
// Depth panics, so a future fourth depth added to this package without a
// matching case here fails loudly instead of silently mapping to nothing.
func ProfileForDepth(d Depth) (profileID string, ok bool) {
	switch d {
	case DepthGate:
		return "", false
	case DepthFullPipeline:
		return "autopilot_with_pr", true
	case DepthShortFlow:
		return "autopilot_with_pr_no_planning", true
	}
	panic(fmt.Sprintf("KERNL DISPATCH FAILURE: depth %q has no profile mapping - Fix: add a case to ProfileForDepth in internal/dispatch/depth_profile.go", d))
}
