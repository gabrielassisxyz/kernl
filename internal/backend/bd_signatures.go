package backend

// bd CLI signature constants used by retry and recovery logic in bdcli.go.
//
// These strings are coupled to the bd binary version pinned via expectedBdVersion.
// When upgrading bd, re-verify each signature by triggering the corresponding
// error condition manually or via the tagged integration test in
// bd_signatures_integration_test.go. A signature mismatch will cause the
// associated retry/recovery path to silently stop firing, leading to confusing
// hangs or failures in the orchestrator.
//
// Reproduction recipes (also documented in the integration test):
//
//	outOfSyncSignature:
//	  Modify .beads/issues.jsonl directly without going through bd, then run
//	  any mutating bd command. bd detects the checksum mismatch and emits this
//	  string.
//
//	doltNilPanicSignature / doltPanicStackSignature:
//	  Trigger the embedded Dolt engine panic by interrupting a write mid-flight
//	  (e.g., kill -9 mid-transaction) or by corrupting the Dolt store. Both
//	  signatures must appear together for the panic to be conclusive; the
//	  orchestrator checks either.
//
//	lockWaitTimeoutSig:
//	  Emitted by the orchestrator itself (bdcli.go) when a repo-lock wait
//	  exceeds the configured limit — not by bd directly.
//
//	commandTimeoutSig:
//	  Emitted by the orchestrator itself (bdcli.go) when a bd invocation
//	  exceeds its per-call timeout.
//
//	noDaemonFlag / "unknown flag" pattern:
//	  Run `bd --no-daemon list` against a bd version older than the one that
//	  introduced the flag. bd emits "unknown flag: --no-daemon" to stderr.
const (
	// expectedBdVersion is the bd release this binary was validated against.
	// If the installed version differs, the orchestrator logs a warning with
	// marker KERNL BD VERSION DRIFT. Execution continues — this is advisory.
	expectedBdVersion = "1.0.4"

	outOfSyncSignature      = "Database out of sync with JSONL"
	noDaemonFlag            = "--no-daemon"
	bdAppendNotesFlag       = "--append-notes"
	bdNoDBEnv               = "BD_NO_DB"
	doltNilPanicSignature   = "panic: runtime error: invalid memory address or nil pointer dereference"
	doltPanicStackSignature = "SetCrashOnFatalError"
	lockWaitTimeoutSig      = "Timed out waiting for bd repo lock"
	commandTimeoutSig       = "bd command timed out after"
)
