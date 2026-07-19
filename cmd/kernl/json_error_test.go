package main

import (
	"encoding/json"
	"errors"
	"testing"
)

// R2-002: with --json set, a failing command must emit a JSON error envelope on
// stdout instead of empty stdout + prose on stderr. main() wires wantsJSONOutput
// + errorEnvelope; both are unit-tested here.

func TestWantsJSONOutput(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"task", "list", "--json"}, true},
		{[]string{"--json", "task", "list"}, true},
		{[]string{"task", "list"}, false},
		// After the end-of-flags sentinel, --json is payload, not a flag.
		{[]string{"capture", "--", "note about --json"}, false},
		{[]string{"capture", "--", "--json"}, false},
	}
	for _, c := range cases {
		if got := wantsJSONOutput(c.args); got != c.want {
			t.Errorf("wantsJSONOutput(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestErrorEnvelopeIsParseableAndCarriesExitCode(t *testing.T) {
	// usagef errors exit 2; a plain error exits 1.
	env := errorEnvelope(usagef("KERNL DISPATCH FAILURE: bad flag"))
	var got struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		ExitCode int    `json:"exitCode"`
	}
	if err := json.Unmarshal([]byte(env), &got); err != nil {
		t.Fatalf("envelope must be valid JSON, got %q (%v)", env, err)
	}
	if got.OK {
		t.Error("a failure envelope must have ok:false")
	}
	if got.ExitCode != 2 {
		t.Errorf("usage error must carry exitCode 2, got %d", got.ExitCode)
	}
	if got.Error == "" {
		t.Error("envelope must carry the error message")
	}

	plain := errorEnvelope(errors.New("boom"))
	var p struct {
		ExitCode int `json:"exitCode"`
	}
	_ = json.Unmarshal([]byte(plain), &p)
	if p.ExitCode != 1 {
		t.Errorf("a plain runtime error must carry exitCode 1, got %d", p.ExitCode)
	}
}

// An alreadyReported error (doctor --json wrote its own output) must be
// detectable by main so it skips the envelope, and must still expose the
// wrapped error's exit code.
func TestAlreadyReportedIsDetectableAndKeepsExitCode(t *testing.T) {
	err := reportedElsewhere(errors.New("checks failed"))
	var reported alreadyReported
	if !errors.As(err, &reported) {
		t.Fatal("main must be able to detect an alreadyReported error")
	}
	if exitCode(err) != 1 {
		t.Errorf("a reported runtime error still exits 1, got %d", exitCode(err))
	}
	// A wrapped usage error keeps exit 2.
	if exitCode(reportedElsewhere(usagef("bad"))) != 2 {
		t.Error("alreadyReported must not mask the wrapped usage exit code")
	}
}
