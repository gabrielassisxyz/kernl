// Package tagname normalises and validates tag names.
//
// Tags nest by convention: the name is a flat string whose `/` separators are
// read as a hierarchy at query time (`homelab/nas` is a child of `homelab`).
// That convention only holds if every write path agrees on what a name looks
// like, and there are two of them — tags.Add and the nodes chokepoint. The
// chokepoint cannot import tags (tags already imports nodes), so the shared
// rules live here, in a package that imports nothing from the graph.
package tagname

import (
	"errors"
	"fmt"
	"strings"
)

// Separator nests a tag under a parent.
const Separator = "/"

// ErrInvalid is the sentinel behind every rejection here, so callers can map a
// bad tag name to a 400 without matching on the message.
var ErrInvalid = errors.New("tagname: invalid tag name")

// Normalize returns the canonical form of a tag name: trimmed, lowercased, and
// with each `/`-separated segment trimmed. Lowercasing is what makes tags a
// matching axis rather than prose — tags.name is UNIQUE, so without it
// `Homelab` and `homelab` would be two subjects that never meet.
//
// It rejects names that would break the nesting convention: empty or
// whitespace-only names, a leading or trailing separator, and empty segments
// (`foo//bar`). Every error wraps ErrInvalid.
func Normalize(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalid)
	}

	segments := strings.Split(strings.ToLower(trimmed), Separator)
	for i, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return "", fmt.Errorf("%w: %q has an empty segment", ErrInvalid, name)
		}
		segments[i] = seg
	}
	return strings.Join(segments, Separator), nil
}

// Validate reports whether name is a well-formed tag name.
func Validate(name string) error {
	_, err := Normalize(name)
	return err
}

// NormalizeAll normalises every name, preserving order and dropping duplicates
// that only differed by case or padding. It fails on the first invalid name.
func NormalizeAll(names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		normalized, err := Normalize(name)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

// ValidateAll returns the first error among names, if any.
func ValidateAll(names []string) error {
	_, err := NormalizeAll(names)
	return err
}

// Segments splits a normalised name into its hierarchy levels.
func Segments(name string) []string {
	return strings.Split(name, Separator)
}

// IsUnder reports whether name is ancestor itself or one of its descendants.
// It is the in-memory twin of the SQL prefix match in tags.NodesUnder, and it
// exists so a caller filtering an already-loaded tag list cannot drift from the
// query semantics: `home` must not match `homelab`.
func IsUnder(name, ancestor string) bool {
	return name == ancestor || strings.HasPrefix(name, ancestor+Separator)
}
