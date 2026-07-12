package tags

import (
	"errors"
	"strings"
)

// SystemPrefix namespaces machine-authored tags. Tags are a user-facing
// navigation axis, so anything the system writes for its own bookkeeping lives
// under this prefix and is hidden from tag surfaces by default. It rides the
// `/` nesting convention rather than a schema column, so the same "hide a
// subtree" rule that powers nesting also hides system tags.
const SystemPrefix = "sys/"

// The machine-authored tags. These were flat, unprefixed literals scattered
// across the inbox, audit and ingest packages; they are named here so a writer
// and its reader can never drift apart.
const (
	// Capture lifecycle, written and read by internal/inbox.
	Pending   = SystemPrefix + "pending"
	Triaged   = SystemPrefix + "triaged"
	Discarded = SystemPrefix + "discarded"

	// Autonomous-decision audit trail, written by internal/dispatch and read by
	// the audit API.
	Audit      = SystemPrefix + "audit"
	Autonomous = SystemPrefix + "autonomous"

	// Marks the bookmark standing in for an ingested source document.
	IngestSource = SystemPrefix + "ingest-source"
)

// ErrSystemTag is returned when a user-supplied tag lands in the reserved
// system namespace.
var ErrSystemTag = errors.New("tags: the \"" + SystemPrefix + "\" prefix is reserved for system tags")

// IsSystem reports whether name is a system tag. It tolerates the casing and
// padding a hand-authored tag arrives with, so the check cannot be sidestepped
// by typing " Sys/pending".
func IsSystem(name string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), SystemPrefix)
}

// RejectSystem returns ErrSystemTag if any name is in the system namespace. It
// guards the boundaries where tags arrive from a human: the API request body
// and the vault's YAML frontmatter.
func RejectSystem(names []string) error {
	for _, n := range names {
		if IsSystem(n) {
			return ErrSystemTag
		}
	}
	return nil
}
