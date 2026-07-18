package main

import (
	"errors"
	"fmt"
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
