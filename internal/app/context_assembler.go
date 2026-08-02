package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// DefaultContextDocs is what AssembleContext reads when a repository
// declares no registry.repos[].contextDocs of its own. README first, so a
// repository that wants to explain itself controls the leading text with
// its own file; ROADMAP.md second, because it is the one file this project
// itself deliberately does not publish (AGENTS.md §6) and the assembler has
// to say so plainly rather than silently rendering a shorter prompt.
var DefaultContextDocs = []string{"README.md", "ROADMAP.md"}

// contextBudgetBytes bounds AssembleContext's total output. It is a loud
// backstop against a misconfigured contextDocs list, not a design budget -
// the selection of what to list is the operator's - so hitting it also logs
// a slog.Warn naming the file that got cut, rather than silently handing
// the Oracle a worse prompt.
const contextBudgetBytes = 32 * 1024

// AssembleContext renders the text of docs - repository-relative paths, in
// order - from the repository at repoPath, for an actor that has no other
// way to see what this repository is for (see Unit A of
// local/artifacts/plans/2026-08-01-composer-context-and-fork-gate-plan.md
// §2.3). A path that does not resolve to a file - because the repository
// does not have one, not because something is broken - renders an explicit
// line saying so, never silence: an omitted section reads, to a reader of
// the prompt, exactly like this feature never having run at all (the same
// rule renderRelatedDecisions in prompt.go follows, for the same reason).
//
// Hermetic: it only ever reads files under repoPath, never the network or a
// subprocess, so it is safe to call from a unit test against t.TempDir().
func AssembleContext(repoPath string, docs []string) string {
	var out strings.Builder
	remaining := contextBudgetBytes
	for _, rel := range docs {
		if remaining <= 0 {
			// The budget was already spent by an earlier doc in this list -
			// possibly spent EXACTLY, with nothing left over. Either way this
			// entry still gets a line naming it as dropped: silently omitting
			// it would read, to a reader of the prompt, exactly like it was
			// never configured at all.
			fmt.Fprintf(&out, "### %s\n\n`%s` was dropped: the %d KB context budget was already exhausted by an earlier entry.\n\n", rel, rel, contextBudgetBytes/1024)
			slog.Warn("KERNL context assembler dropped a contextDocs entry: the budget was already exhausted", "file", rel, "repoPath", repoPath, "budgetBytes", contextBudgetBytes)
			continue
		}
		section := renderContextDoc(repoPath, rel)
		if len(section) <= remaining {
			out.WriteString(section)
			remaining -= len(section)
			continue
		}
		cut := cutOnLineBoundary(section, remaining)
		out.WriteString(cut)
		fmt.Fprintf(&out, "\n[... %s truncated: the %d KB context budget was exceeded ...]\n", rel, contextBudgetBytes/1024)
		slog.Warn("KERNL context assembler truncated a file to fit the context budget", "file", rel, "repoPath", repoPath, "budgetBytes", contextBudgetBytes)
		remaining = 0
	}
	return out.String()
}

// renderContextDoc renders one file as its own subsection: a heading naming
// it, then its content. Four cases, each rendered as an explicit line rather
// than silently contributing nothing or a misleading one:
//
//   - the entry resolves outside repoPath (a misconfigured "../" or similar):
//     rejected loudly, naming the entry and the config key that fixes it.
//   - the file does not exist: the normal case for an unpublished doc (e.g.
//     kernl's own ROADMAP.md, AGENTS.md §6) - not a failure.
//   - any other read error (a directory, a permission error, I/O): a genuine
//     failure of a configured resource (AGENTS.md §2), logged loud rather
//     than folded into the same sentence as the "does not exist" case.
//   - the file exists and is empty: said plainly, rather than leaving a
//     heading with a blank body for the reader to puzzle over.
func renderContextDoc(repoPath, rel string) string {
	full, ok := contextDocWithinRepo(repoPath, rel)
	if !ok {
		slog.Warn("KERNL DISPATCH FAILURE: contextDocs entry resolves outside the repository", "file", rel, "repoPath", repoPath)
		return fmt.Sprintf("### %s\n\n`%s` was rejected: it resolves outside this repository - Fix: correct registry.repos[].contextDocs.\n\n", rel, rel)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("### %s\n\n`%s` was not found in this repository - it publishes none.\n\n", rel, rel)
		}
		slog.Warn("KERNL DISPATCH FAILURE: contextDocs entry could not be read", "file", rel, "repoPath", repoPath, "error", err)
		return fmt.Sprintf("### %s\n\n`%s` could not be read: %v.\n\n", rel, rel, err)
	}
	if len(data) == 0 {
		return fmt.Sprintf("### %s\n\n`%s` is present but empty.\n\n", rel, rel)
	}
	return fmt.Sprintf("### %s\n\n%s\n\n", rel, strings.TrimRight(string(data), "\n"))
}

// contextDocWithinRepo joins repoPath and rel and confirms the cleaned
// result stays inside repoPath, rejecting an entry like "../private.md" that
// would otherwise read a sibling directory and forward it verbatim to
// whatever llm.agent/llm.endpoint points at. This is operator-written
// config, not hostile input, so it guards against a typo rather than an
// attack - which is also why an ABSOLUTE rel needs no special case:
// filepath.Join treats a second absolute argument as just another path
// segment (filepath.Join("/repo", "/etc/passwd") == "/repo/etc/passwd"), so
// it is already contained.
func contextDocWithinRepo(repoPath, rel string) (string, bool) {
	root := filepath.Clean(repoPath)
	full := filepath.Join(root, rel)
	if full == root || strings.HasPrefix(full, root+string(filepath.Separator)) {
		return full, true
	}
	return "", false
}

// cutOnLineBoundary returns the largest prefix of s that fits within limit
// bytes and ends exactly at a newline, so a cut never leaves a partial line
// dangling in front of the marker AssembleContext appends after it. A limit
// too small to hold even one full line drops the section entirely rather
// than emit a partial one.
func cutOnLineBoundary(s string, limit int) string {
	if limit >= len(s) {
		return s
	}
	head := s[:limit]
	if idx := strings.LastIndexByte(head, '\n'); idx >= 0 {
		return head[:idx+1]
	}
	return ""
}
