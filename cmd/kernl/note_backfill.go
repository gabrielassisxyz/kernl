package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// backfillEntity is one entity the sweep reports, mirroring the API's shape.
type backfillEntity struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Path  string `json:"path"`
}

type backfillReport struct {
	DryRun   bool             `json:"dryRun"`
	Entities []backfillEntity `json:"entities"`
}

// runNoteBackfillCompanions writes the missing companion note for every task,
// project and bookmark that has none.
//
// The dry run is the default and --yes is required to write, which is the same
// gate `kernl note delete` and `kernl sweep` use, for a reason particular to this
// command: the sweep cannot tell an entity that never had a companion from one
// whose note was deleted on purpose, so a run without confirmation would
// resurrect notes the user threw away.
func runNoteBackfillCompanions(ctx context.Context, c *apiClient, out io.Writer, asJSON bool, args []string) error {
	confirmed, rest := parseBoolFlag(args, "--yes")
	// --dry-run is accepted and is the default, so the safe spelling is also the
	// explicit one: a script that passes it keeps working if the default ever moves.
	_, rest = parseBoolFlag(rest, "--dry-run")
	if err := rejectUnknownFlags("note backfill-companions", rest); err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: note backfill-companions takes no arguments, got %d - it always sweeps the whole vault", len(rest))
	}

	var raw json.RawMessage
	var err error
	if confirmed {
		raw, err = c.postRaw(ctx, "/api/vault/companions/backfill", "application/json", []byte("{}"))
	} else {
		raw, err = c.get(ctx, "/api/vault/companions/missing")
	}
	if err != nil {
		return err
	}
	if asJSON {
		return emitJSON(out, raw)
	}

	var report backfillReport
	if err := decodeInto(raw, "companion backfill", &report); err != nil {
		return err
	}
	if len(report.Entities) == 0 {
		_, err := fmt.Fprintln(out, "every task, project and bookmark already has a companion note")
		return err
	}

	for _, e := range report.Entities {
		if e.Path != "" {
			if _, err := fmt.Fprintf(out, "wrote %s\t%s\t%s\n", e.Path, e.Type, e.Title); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", e.ID, e.Type, e.Title); err != nil {
			return err
		}
	}
	if !report.DryRun {
		_, err := fmt.Fprintf(out, "\n%d companion note(s) written.\n", len(report.Entities))
		return err
	}
	_, err = fmt.Fprintf(out,
		"\n%d entity/entities have no companion note. Re-run with --yes to write them.\nA companion you deleted by hand will come back: this cannot tell that from one that never existed.\n",
		len(report.Entities))
	return err
}
