package backend

import (
	"fmt"
	"strings"
)

// CloseRefusalKind identifies which ordering constraint br enforced when it
// refused a close - the two situations NOTHING_TO_DO covers once "already
// closed" is ruled out by recoverNoOpClose's re-read.
type CloseRefusalKind int

const (
	CloseRefusalUnspecified CloseRefusalKind = iota
	// CloseRefusalOpenChildren is an epic close refused because one of its
	// child beads (a parent-child dependency) is still open.
	CloseRefusalOpenChildren
	// CloseRefusalOpenDependents is a bead close refused because a bead it
	// depends on (a "blocks" dependency) is still open - the refusal a
	// dependency chain hits at every level but the last.
	CloseRefusalOpenDependents
)

// CloseRefusedError reports that Close's target is still open because br's
// own ordering guard refused it, not because the tracker itself failed. A
// caller that wants to retry it once the blocker closes - sweep's
// convergence loop - recovers it with errors.As rather than matching text.
// Reason never carries br's "--force" suggestion: --force disables the very
// guard this error exists to report, so repeating it here would point an
// automated caller at the one action it must never take on its own.
type CloseRefusedError struct {
	ID     string
	Kind   CloseRefusalKind
	Reason string
}

func (e *CloseRefusedError) Error() string {
	return fmt.Sprintf("KERNL DISPATCH FAILURE: br close %s refused: %s", e.ID, e.Reason)
}

// classifyCloseRefusal reads br's own envelope hint - a fixed, two-sentence
// shape, not the free-form per-issue reason - to tell an epic's
// open-children refusal apart from a bead's own open-dependency refusal. A
// hint matching neither is not a recognised ordering refusal: the caller
// keeps br's original text untouched rather than guess.
func classifyCloseRefusal(hint string) (CloseRefusalKind, bool) {
	switch {
	case strings.Contains(hint, "open children"):
		return CloseRefusalOpenChildren, true
	case strings.Contains(hint, "open blocking dependencies"):
		return CloseRefusalOpenDependents, true
	default:
		return CloseRefusalUnspecified, false
	}
}

// redactForceHint removes br's own "--force" suggestion from a refusal
// reason already classified above. The suggestion is introduced differently
// per kind ("(use --force...)" on an open-children refusal, "... or use
// --force..." on an open-dependency one), so the marker searched for differs
// too; a kind this does not recognise is left untouched, matching
// classifyCloseRefusal's own "leave it alone" default.
func redactForceHint(kind CloseRefusalKind, reason string) string {
	var marker string
	switch kind {
	case CloseRefusalOpenChildren:
		marker = " (use --force"
	case CloseRefusalOpenDependents:
		marker = " \u2014 close the open blocker(s) first, or use --force"
	default:
		return reason
	}
	if i := strings.Index(reason, marker); i >= 0 {
		return reason[:i]
	}
	return reason
}
