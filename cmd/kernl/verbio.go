package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// stdinIsTerminal reports whether stdin is an interactive terminal (a char
// device) rather than a pipe, redirect or file. A blocking io.ReadAll on a
// terminal with no input hangs forever with no prompt — the worst trap for an
// agent that forgot to pipe input — so the stdin-reading verbs check this and
// fail fast with a usage error. It is a package var so a test can force the
// terminal branch without allocating a pty.
var stdinIsTerminal = func() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// emitJSON writes an API response through untouched. The REST layer already
// speaks camelCase, so passing the server's own body along keeps the CLI's
// --json contract identical to the API's — no second mapping to drift.
func emitJSON(w io.Writer, raw json.RawMessage) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		// Not JSON (an empty 204 body, say) — pass the bytes through as-is.
		_, err := fmt.Fprintln(w, strings.TrimSpace(string(raw)))
		return err
	}
	_, err := fmt.Fprintln(w, pretty.String())
	return err
}

// createdLine renders one creation confirmation: what you named first and in
// quotes, the id last and in parentheses.
//
// WHY this shape. Graph node ids are bare UUIDv7 — no readable prefix, nothing a
// caller can check against what they typed. Leading with the id therefore opens
// every confirmation with the one field carrying no information. The quotes are
// load-bearing rather than decorative: several of these verbs join unquoted
// positional words into the title, and unquoted output makes a swallowed or
// stray word invisible. Staying on one line keeps grep and awk working; a
// structured consumer uses --json, which this does not touch.
// lead is the action and the thing ("Created task"), tail is any extra context
// that belongs between the name and the id ("under \"habits\"") or empty.
func createdLine(lead, name, tail, id string) string {
	if tail != "" {
		return fmt.Sprintf("%s %q %s (%s)", lead, name, tail, id)
	}
	return fmt.Sprintf("%s %q (%s)", lead, name, id)
}

// decodeInto unmarshals an API response, naming the route in the error so a
// contract change reads as a contract change rather than a parse failure.
func decodeInto(raw json.RawMessage, route string, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return wrapLoud(fmt.Sprintf("decoding the response of %s", route), err)
	}
	return nil
}

// takeFlag strips a "--name value" / "--name=value" pair from args and reports
// whether it was present, so a verb can tell "flag omitted" from "flag set to
// the empty string" (the difference between leaving a field alone and clearing
// it in a PATCH).
//
// verb is the full subcommand path ("task create"), used only to point a
// dangling flag at the help that documents it. Root 'kernl --help' lists the
// verbs and none of their flags, so it cannot answer the question the caller
// just got wrong; the owning verb's help can.
func takeFlag(verb string, args []string, name string) (value string, present bool, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == name:
			if i+1 >= len(args) {
				return "", false, nil, usagef("KERNL DISPATCH FAILURE: %s requires a value — run: kernl %s --help", name, verb)
			}
			value, present = args[i+1], true
			i++
		case strings.HasPrefix(a, name+"="):
			value, present = strings.TrimPrefix(a, name+"="), true
		default:
			rest = append(rest, a)
		}
	}
	return value, present, rest, nil
}

// rejectUnknownFlags fails loud on any leftover flag-looking argument. Without
// it a typo'd flag is silently treated as a positional argument, which is how
// a wrong invocation ends up doing the wrong thing quietly.
func rejectUnknownFlags(verb string, args []string) error {
	for _, a := range args {
		if strings.HasPrefix(a, "-") && a != "-" {
			return usagef("KERNL DISPATCH FAILURE: unknown flag %q for %s — run: kernl %s --help", a, verb, verb)
		}
	}
	return nil
}

// requireSub validates a subcommand against the closed set a verb accepts.
func requireSub(verb string, args []string, valid []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, usagef("KERNL DISPATCH FAILURE: %s requires a subcommand — valid: %s. Run: kernl %s --help",
			verb, strings.Join(valid, ", "), verb)
	}
	for _, v := range valid {
		if args[0] == v {
			return args[0], args[1:], nil
		}
	}
	return "", nil, usagef("KERNL DISPATCH FAILURE: unknown %s subcommand %q%s — valid: %s. Run: kernl %s --help",
		verb, args[0], didYouMean(args[0], valid), strings.Join(valid, ", "), verb)
}
