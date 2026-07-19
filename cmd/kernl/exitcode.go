package main

import (
	"errors"
	"fmt"
	"strings"
)

// Exit-code dictionary — the CLI's contract with scripts and agents:
//
//	0  success
//	1  runtime/internal error (backend, config load, network, agent run)
//	2  usage error (unknown verb/flag, missing argument, bad value)
//
// Callers can branch on `case $? in 2) fix the invocation;; 1) retry/inspect;;`.
type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }
func (u usageError) Unwrap() error { return u.err }

// usagef builds an error that exits with code 2 (usage) instead of 1.
func usagef(format string, a ...any) error {
	return usageError{err: fmt.Errorf(format, a...)}
}

// alreadyReported marks an error whose output the verb has already written — e.g.
// `doctor --json` prints its own report to stdout and then signals failure only
// through the exit code. main uses the exit code but prints nothing more, so the
// output is not duplicated by the stderr / JSON-envelope path. The wrapped error
// still drives exitCode (usage vs runtime) through Unwrap.
type alreadyReported struct{ err error }

func (a alreadyReported) Error() string { return a.err.Error() }
func (a alreadyReported) Unwrap() error { return a.err }

// reportedElsewhere wraps an error the verb has already surfaced itself.
func reportedElsewhere(err error) error { return alreadyReported{err: err} }

// wrapLoud prefixes context onto an error, adding the KERNL DISPATCH FAILURE
// marker only when the wrapped error does not already carry it — the marker
// must appear exactly once per error chain.
func wrapLoud(context string, err error) error {
	if strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		return fmt.Errorf("%s: %w", context, err)
	}
	return fmt.Errorf("KERNL DISPATCH FAILURE: %s: %w", context, err)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var u usageError
	if errors.As(err, &u) {
		return 2
	}
	return 1
}
