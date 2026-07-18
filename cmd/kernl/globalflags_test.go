package main

import (
	"strings"
	"testing"
)

func TestParseStringFlagAcceptsGnuEqualsForm(t *testing.T) {
	v, rest, err := parseConfigPath([]string{"--config=x.yaml", "doctor"})
	if err != nil || v != "x.yaml" || len(rest) != 1 || rest[0] != "doctor" {
		t.Fatalf("--config=x.yaml: v=%q rest=%v err=%v", v, rest, err)
	}
	v, rest, err = parseConfigPath([]string{"--config", "y.yaml", "doctor"})
	if err != nil || v != "y.yaml" || len(rest) != 1 {
		t.Fatalf("--config y.yaml: v=%q rest=%v err=%v", v, rest, err)
	}
}

func TestParseStringFlagDuplicateLastWins(t *testing.T) {
	v, rest, err := parseConfigPath([]string{"--config", "a.yaml", "--config", "b.yaml", "doctor"})
	if err != nil || v != "b.yaml" {
		t.Fatalf("duplicate --config must be last-wins, got v=%q err=%v", v, err)
	}
	if len(rest) != 1 || rest[0] != "doctor" {
		t.Fatalf("both occurrences must be stripped, rest=%v", rest)
	}
}

func TestParseStringFlagMissingValueFailsLoud(t *testing.T) {
	_, _, err := parseConfigPath([]string{"doctor", "--config"})
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("--config without value must be usage error, got: %v", err)
	}
}

func TestParsePortInvalidValueFailsLoud(t *testing.T) {
	// A non-numeric --port used to be silently dropped, falling back to the
	// default port with no diagnostic at all.
	_, _, err := parsePort([]string{"--port", "abc", "serve"})
	if err == nil || !strings.Contains(err.Error(), `got "abc"`) {
		t.Fatalf("--port abc must fail naming the value, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("invalid port is a usage error, got exit %d", exitCode(err))
	}
	p, rest, err := parsePort([]string{"--port=9090", "serve"})
	if err != nil || p != 9090 || len(rest) != 1 {
		t.Fatalf("--port=9090: p=%d rest=%v err=%v", p, rest, err)
	}
}
