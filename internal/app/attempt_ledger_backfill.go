package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

// LedgerBackfill is one ledger file the backfill would change, or did.
type LedgerBackfill struct {
	Path string
	// Marked counts rows that gain causedBy: they follow a deliberate
	// rejection and were never recorded as the rework they are. Unmarked
	// counts the opposite - rows carrying a causedBy derived from
	// verdict_not_pass, which never sent work anywhere.
	Marked   int
	Unmarked int
	// DanglingDropped reports that the file ended in an incomplete row,
	// which this pass trims exactly as an append would have.
	DanglingDropped bool
}

// CausedByBackfillResult is BackfillCausedBy's answer. Ledgers lists only the
// files with something to change; a file already consistent with the current
// rule is counted in LedgersScanned and nothing more, so the report is the
// work to do rather than an inventory of everything that exists.
type CausedByBackfillResult struct {
	Applied        bool
	LedgersScanned int
	RowsScanned    int
	Ledgers        []LedgerBackfill
}

// BackfillCausedBy re-derives causedBy across every recorded attempt, under
// the rule findCausedBy applies today.
//
// It exists because causedBy is computed once, at write time, from the
// ledger's own history - so rows written while the derivation matched
// verdict_not_pass keep that answer forever, and no reader can tell them
// apart from rows written under the corrected rule. A rework number built on
// a mixture of the two describes neither period.
//
// apply=false changes nothing and reports what would change: the default,
// because this rewrites the only record kernl keeps of what its agents did.
//
// The rewrite holds the same exclusive flock appendStageAttempt takes, on the
// same file, so a live orchestrator appending mid-pass blocks rather than
// interleaving. It is deliberately in-place rather than write-temp-and-rename:
// an appender that opened the file before blocking on the lock holds a
// descriptor to that inode, and renaming a new file over the path would send
// its row to an inode nobody will ever read again.
func BackfillCausedBy(stateDir string, apply bool) (CausedByBackfillResult, error) {
	paths, err := AttemptLedgerPaths(stateDir)
	if err != nil {
		return CausedByBackfillResult{}, err
	}

	result := CausedByBackfillResult{Applied: apply, LedgersScanned: len(paths)}
	for _, path := range paths {
		outcome, rows, err := backfillOneLedger(path, apply)
		if err != nil {
			return CausedByBackfillResult{}, err
		}
		result.RowsScanned += rows
		if outcome.Marked > 0 || outcome.Unmarked > 0 || outcome.DanglingDropped {
			result.Ledgers = append(result.Ledgers, outcome)
		}
	}
	return result, nil
}

func backfillOneLedger(path string, apply bool) (LedgerBackfill, int, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return LedgerBackfill{}, 0, fmt.Errorf("KERNL DISPATCH FAILURE: opening attempt ledger %s to recompute causedBy: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if lockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); lockErr != nil {
		return LedgerBackfill{}, 0, fmt.Errorf("KERNL DISPATCH FAILURE: locking attempt ledger %s to recompute causedBy: %w", path, lockErr)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	data, err := io.ReadAll(f)
	if err != nil {
		return LedgerBackfill{}, 0, fmt.Errorf("KERNL DISPATCH FAILURE: reading attempt ledger %s: %w", path, err)
	}
	records, validSize, err := parseLedgerBytes(path, data)
	if err != nil {
		return LedgerBackfill{}, 0, err
	}

	outcome := LedgerBackfill{Path: path, DanglingDropped: validSize != int64(len(data))}
	rebuilt, changed, err := rebuildLedgerLines(string(data[:validSize]), records, &outcome)
	if err != nil {
		return LedgerBackfill{}, 0, fmt.Errorf("KERNL DISPATCH FAILURE: rewriting attempt ledger %s: %w", path, err)
	}

	if !apply || (!changed && !outcome.DanglingDropped) {
		return outcome, len(records), nil
	}
	if err := writeLedgerInPlace(f, path, rebuilt); err != nil {
		return LedgerBackfill{}, 0, err
	}
	return outcome, len(records), nil
}

// rebuildLedgerLines returns the file's new content, keeping every unchanged
// row's original bytes verbatim. Only a row whose causedBy actually moves is
// re-encoded, which keeps the rewrite auditable: a diff of the ledger after
// this pass shows the rows it decided about and nothing else.
//
// A re-encoded row goes through a generic map rather than StageAttemptRecord.
// Unmarshalling into the struct and marshalling back would silently drop any
// field the struct no longer has - and every row here was written by an older
// build of this program, which is precisely the population most likely to
// carry one.
func rebuildLedgerLines(valid string, records []StageAttemptRecord, outcome *LedgerBackfill) (string, bool, error) {
	var b strings.Builder
	changed := false
	seen := make([]StageAttemptRecord, 0, len(records))
	next := 0

	for _, seg := range strings.Split(valid, "\n") {
		if strings.TrimSpace(seg) == "" {
			// Not the trailing empty segment after the final newline: that
			// one carries no newline of its own to write back.
			continue
		}
		rec := records[next]
		next++

		want := findCausedBy(seen, rec.BeadID, rec.Stage)
		seen = append(seen, rec)

		if sameCausedBy(rec.CausedBy, want) {
			b.WriteString(seg)
			b.WriteByte('\n')
			continue
		}
		if want == nil {
			outcome.Unmarked++
		} else {
			outcome.Marked++
		}
		changed = true

		rewritten, err := replaceCausedBy(seg, want)
		if err != nil {
			return "", false, err
		}
		b.WriteString(rewritten)
		b.WriteByte('\n')
	}
	return b.String(), changed, nil
}

func sameCausedBy(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// replaceCausedBy sets causedBy on one raw row, leaving every other key it
// finds untouched. Encoding a Go map sorts the keys, so a rewritten row is
// ordered differently from one the writer produced - that is cosmetic, and it
// is the price of not deciding, on the operator's behalf, that a field this
// build does not know about was not worth keeping.
func replaceCausedBy(line string, causedBy *string) (string, error) {
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		return "", fmt.Errorf("re-encoding a row that already parsed as a record: %w", err)
	}
	if causedBy == nil {
		row["causedBy"] = nil
	} else {
		row["causedBy"] = *causedBy
	}
	out, err := json.Marshal(row)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// writeLedgerInPlace replaces the file's contents with rebuilt, keeping a
// copy of the original beside it until the write is known to have landed.
// The append path can repair a torn trailing row because that is the only
// state it ever leaves; this pass rewrites the middle of the file, where a
// partial write is unrecoverable and the data exists nowhere else - the graph
// db is not versioned and neither is this.
func writeLedgerInPlace(f ledgerFile, path, rebuilt string) error {
	backup := path + ".before-backfill"
	original, err := readAllFrom(f)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: re-reading attempt ledger %s before rewriting it: %w", path, err)
	}
	if err := os.WriteFile(backup, original, 0o644); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing the safety copy %s before rewriting the attempt ledger: %w", backup, err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: seeking attempt ledger %s: %w", path, err)
	}
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: truncating attempt ledger %s: %w", path, err)
	}
	n, err := f.Write([]byte(rebuilt))
	if err == nil && n != len(rebuilt) {
		err = fmt.Errorf("short write: wrote %d of %d bytes", n, len(rebuilt))
	}
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: rewriting attempt ledger %s: %w - the file it replaced is intact at %s, restore it with: mv %s %s", path, err, backup, backup, path)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: the attempt ledger %s was rewritten, but its safety copy %s could not be removed: %w - delete it by hand, it is a full copy of the ledger as it was", path, backup, err)
	}
	return nil
}

func readAllFrom(f ledgerFile) ([]byte, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}
