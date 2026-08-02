package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// runIndexHeaderPrefix marks the first line of an epic's report.md once it
// holds the run index updateRunIndex writes, rather than a single run's own
// report - the shape report.md had before per-run files existed (see
// writeRunReport's own doc comment in run_report.go). Any report.md whose
// first line does not start with this prefix predates the split and is
// archived, never overwritten - see archiveLegacyReport.
const runIndexHeaderPrefix = "# Run Report Index: "

// updateRunIndex folds one line describing this run into the epic's
// report.md, under an exclusive advisory flock on that file - the same
// mechanism AppendStageAttempt (attempt_ledger.go) uses to serialize two
// kernl processes (or two goroutines) that close a run over the same epic at
// close to the same moment, and for the same reason: a mutex keyed on a Go
// string only ever serializes callers inside one process, and this file is
// read, decided over, and rewritten whole - not merely appended to, since a
// new run's line belongs above every earlier one.
//
// Unlike AppendStageAttempt's JSONL ledger, an interrupted rewrite here has
// no self-repair: report.md is a convenience index over the real record
// (each run's own file under runs/), never the sole copy of anything a
// crash could lose, so a truncated index left by a rewrite that died
// mid-write costs nothing a human cannot see by listing runs/ directly -
// the recovery machinery AppendStageAttempt needs to protect its one
// authoritative copy would be solving a problem this file does not have.
func updateRunIndex(in ComposeRunReportInput, decisionCount int) (err error) {
	epicDir, err := resolveEpicDir(in.StateDir, in.EpicID)
	if err != nil {
		return err
	}
	indexPath := filepath.Join(epicDir, "report.md")

	f, err := os.OpenFile(indexPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: opening run index %s: %w", indexPath, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("KERNL DISPATCH FAILURE: closing run index %s after write: %w", indexPath, cerr)
		}
	}()

	if lockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); lockErr != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: locking run index %s: %w", indexPath, lockErr)
	}
	defer func() {
		if uerr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); uerr != nil && err == nil {
			err = fmt.Errorf("KERNL DISPATCH FAILURE: unlocking run index %s: %w", indexPath, uerr)
		}
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: reading run index %s: %w", indexPath, err)
	}

	priorEntries, legacy := parseRunIndex(data)
	var legacyNote string
	if legacy != nil {
		archivedRel, archErr := archiveLegacyReport(epicDir, legacy)
		if archErr != nil {
			return archErr
		}
		legacyNote = fmt.Sprintf("_A report.md written before per-run reports existed was preserved at %s._\n\n", archivedRel)
	}

	content := renderRunIndex(in.EpicID, legacyNote, renderRunIndexEntry(in, decisionCount), priorEntries)

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: seeking run index %s: %w", indexPath, err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: truncating run index %s: %w", indexPath, err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing run index %s: %w", indexPath, err)
	}
	return nil
}

// parseRunIndex splits an existing report.md into the entry lines a
// previous updateRunIndex call already wrote, or - when the file does not
// even start with runIndexHeaderPrefix - the whole legacy blob, so the
// caller can archive it instead of losing it under this call's rewrite. A
// non-nil legacy return always carries every byte read, since len(data)==0
// (nothing on disk yet) is the only case that returns (nil, nil).
func parseRunIndex(data []byte) (priorEntries []string, legacy []byte) {
	if len(data) == 0 {
		return nil, nil
	}
	text := string(data)
	firstLine := text
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		firstLine = text[:idx]
	}
	if !strings.HasPrefix(firstLine, runIndexHeaderPrefix) {
		return nil, data
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "- ") {
			priorEntries = append(priorEntries, line)
		}
	}
	return priorEntries, nil
}

// archiveLegacyReport preserves a report.md written before per-run reports
// existed, under runs/, rather than letting updateRunIndex's rewrite
// silently discard it - see writeRunReport's own doc comment for why
// report.md changed shape. It never overwrites: legacy.md is tried first,
// and a name collision (most likely because an earlier call already
// migrated this same epic before this one observed the header - two
// dispatches finishing at once) moves on to legacy-1.md, legacy-2.md, and so
// on, so a real legacy file is never clobbered by a second one.
func archiveLegacyReport(epicDir string, legacy []byte) (relPath string, err error) {
	runsDir := filepath.Join(epicDir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating %s to archive a legacy report.md: %w", runsDir, err)
	}

	const maxAttempts = 1000
	for i := 0; i < maxAttempts; i++ {
		name := "legacy.md"
		if i > 0 {
			name = fmt.Sprintf("legacy-%d.md", i)
		}
		target := filepath.Join(runsDir, name)
		f, openErr := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if openErr != nil {
			if os.IsExist(openErr) {
				continue
			}
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: opening %s to archive a legacy report.md: %w", target, openErr)
		}
		_, writeErr := f.Write(legacy)
		closeErr := f.Close()
		if writeErr != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing legacy report.md to %s: %w", target, writeErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: closing %s after archiving a legacy report.md: %w", target, closeErr)
		}
		return filepath.Join("runs", name), nil
	}
	return "", fmt.Errorf("KERNL DISPATCH FAILURE: could not find a free name under %s to archive a legacy report.md after %d attempts", runsDir, maxAttempts)
}

// renderRunIndexEntry is one line of report.md for this run: enough for an
// operator scanning the index in a terminal to decide whether to open
// runs/<RunID>.md, without opening it first. PR is the one column that is
// sometimes absent (shipment never ran, or never got that far) and is
// simply left off rather than printed empty, so a run with no PR yet still
// reads as a clean line instead of a dangling label.
func renderRunIndexEntry(in ComposeRunReportInput, decisionCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- **%s** - %s - %s - %d bead(s) driven - %d decision(s) recorded",
		in.RunID, in.FinishedAt.Format(time.RFC3339), in.Status, len(in.Beads), decisionCount)
	if in.PRURL != "" {
		fmt.Fprintf(&b, " - PR: %s", in.PRURL)
	}
	fmt.Fprintf(&b, " - [report](runs/%s.md)", in.RunID)
	return b.String()
}

// renderRunIndex assembles the whole of report.md: the header naming the
// epic, an optional one-time note pointing at an archived legacy report,
// this run's own new entry, then every prior run's entry carried forward
// unchanged and in the same order they were already in - newest first,
// because every call prepends its own entry rather than appending it.
func renderRunIndex(epicID, legacyNote, newEntry string, priorEntries []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\n\n", runIndexHeaderPrefix, epicID)
	b.WriteString(legacyNote)
	b.WriteString(newEntry)
	b.WriteString("\n")
	for _, e := range priorEntries {
		b.WriteString(e)
		b.WriteString("\n")
	}
	return b.String()
}
