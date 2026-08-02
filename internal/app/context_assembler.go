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
			break
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
// it, then its content - or, when it does not exist, an explicit line
// saying it was not found rather than silently contributing nothing.
func renderContextDoc(repoPath, rel string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, rel))
	if err != nil {
		return fmt.Sprintf("### %s\n\n`%s` was not found in this repository - it publishes none.\n\n", rel, rel)
	}
	return fmt.Sprintf("### %s\n\n%s\n\n", rel, strings.TrimRight(string(data), "\n"))
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
