package planning

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// TelosTag marks a note as Telos — the user's standing identity, values, and
// long-term goals. Telos is relevant to every DA turn, so notes carrying this
// tag are always injected into context rather than retrieved by relevance.
const TelosTag = "telos"

// MaxTelosBytes bounds the total Telos content folded into context so identity
// notes supplement the prompt rather than crowding out the conversation. It is
// exported so surfaces that visualize the injection footprint can show the cap.
const MaxTelosBytes = 4000

const telosHeader = "The user's Telos — their standing identity, values, and long-term goals. " +
	"Treat this as always-relevant context for every response:\n"

// TelosContext is the always-injected identity block plus its injection
// footprint, so a surface can show the user exactly what the DA always sees and
// whether the cap is clipping it.
type TelosContext struct {
	Block     string // the formatted, capped block ("" when there is no Telos)
	Bytes     int    // byte length actually injected
	CapBytes  int    // MaxTelosBytes
	Truncated bool   // true when content exceeded the cap and was cut
}

// LoadTelos returns the user's Telos as a ready-to-inject context block — the
// chat engine's view. It returns "" when there are no Telos notes (or none with
// a body), so callers can inject the result unconditionally. Telos is always-on,
// not relevance-gated — that is what distinguishes it from BuildContext.
func LoadTelos(ctx context.Context, g *graph.Graph) (string, error) {
	tc, err := LoadTelosContext(ctx, g)
	return tc.Block, err
}

// LoadTelosContext loads active `telos`-tagged notes and renders them into the
// always-injected identity block, reporting the injection footprint alongside.
func LoadTelosContext(ctx context.Context, g *graph.Graph) (TelosContext, error) {
	var telos []*nodes.Note
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		// ListNotes excludes tombstoned notes by default, so this is the active set.
		telos, err = nodes.ListNotes(ctx, tx, nodes.NoteFilter{Tags: []string{TelosTag}})
		return err
	})
	if err != nil {
		return TelosContext{CapBytes: MaxTelosBytes}, err
	}
	return renderTelos(telos), nil
}

// renderTelos folds telos notes into the injectable block. Pure (no I/O) so the
// formatting and cap logic have one home shared by every caller.
func renderTelos(telos []*nodes.Note) TelosContext {
	var b strings.Builder
	b.WriteString(telosHeader)
	wrote := false
	for _, n := range telos {
		body := strings.TrimSpace(n.Body)
		if body == "" {
			continue
		}
		b.WriteString("\n## ")
		b.WriteString(strings.TrimSpace(n.Title))
		b.WriteString("\n")
		b.WriteString(body)
		b.WriteString("\n")
		wrote = true
	}
	if !wrote {
		return TelosContext{CapBytes: MaxTelosBytes}
	}

	out := b.String()
	truncated := false
	if len(out) > MaxTelosBytes {
		out = truncateRunes(out, MaxTelosBytes) + "\n…"
		truncated = true
	}
	return TelosContext{Block: out, Bytes: len(out), CapBytes: MaxTelosBytes, Truncated: truncated}
}

// truncateRunes returns s shortened to at most max bytes without splitting a
// multi-byte UTF-8 rune.
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
