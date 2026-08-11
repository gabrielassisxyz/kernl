package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The four envelopes below are every distinct way a pi turn has failed on
// this machine, read from ~/.pi/agent/sessions/*/*.jsonl and copied verbatim.
// They are fixtures rather than paraphrases on purpose: a paraphrase tests
// the classifier against the sentence someone typed into a plan, not against
// the one the provider sends.
const (
	// Six rows on 2026-08-02, all four daytrace-frd beads and one cdw bead.
	// The "RPM limit=15" is itself a timestamp: 15 was the pre-2026-08-02
	// per-deployment cap, so any message quoting it predates the proxy fix.
	measuredRateLimitEnvelope = `429: {"message":"litellm.RateLimitError: Model rate limit exceeded. RPM limit=15, current usage=23. Received Model Group=kimi-k2.7\nAvailable Model Group Fallbacks=None","type":"throttling_error","param":null,"code":"429"}`
	// A model name that does not exist. It will fail identically forever,
	// which is the whole reason this kind has a name.
	measuredInvalidModelEnvelope = `400: {"message":"/chat/completions: Invalid model name passed in model=nao-existe-mesmo. Call ` + "`/v1/models`" + ` to view available models for your key.","type":"None","param":"None","code":"400","provider_specific_fields":{"error":"/chat/completions: Invalid model name passed in model=nao-existe-mesmo."}}`
	// daytrace-frd.19, the one row the batch plan assumed was the same
	// family as the 429s without ever verifying it.
	measuredConnectionError = `Connection error.`
	// An unresolvable environment variable in the provider's header config.
	measuredAuthError = `API key auth failed for provider litellm: Failed to resolve provider "litellm" header "x-litellm-session-id" from environment variable: KERNL_LLM_SESSION_ID`
)

func TestClassifyPiTurnFailure(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		kind FailureKind
		code string
		typ  string
	}{
		{
			name: "the measured rate limit is classified from the payload's code",
			raw:  measuredRateLimitEnvelope,
			kind: FailureRateLimited, code: "429", typ: "throttling_error",
		},
		{
			// Structured does not mean retryable. Without this case the
			// classifier could pass every other test by answering
			// rate_limited to anything it managed to parse.
			name: "the measured invalid model is a request fault, not a rate limit",
			raw:  measuredInvalidModelEnvelope,
			kind: FailureInvalidRequest, code: "400", typ: "None",
		},
		{
			// The assertion the batch plan asks for by name: an unverified
			// assumption must not reach the retry policy that reads this.
			name: "a bare connection error stays unknown",
			raw:  measuredConnectionError,
			kind: FailureUnknown,
		},
		{
			name: "a bare auth failure stays unknown",
			raw:  measuredAuthError,
			kind: FailureUnknown,
		},
		{
			// A failure is already being reported; losing its description to
			// a parse error helps nobody.
			name: "an unparseable payload stays unknown and keeps its text",
			raw:  `429: {"message": "truncated`,
			kind: FailureUnknown,
		},
		{
			name: "a numeric code is the same fact as a quoted one",
			raw:  `429: {"message":"slow down","type":"throttling_error","code":429}`,
			kind: FailureRateLimited, code: "429", typ: "throttling_error",
		},
		{
			// Not measured here, and a guess about a server fault would be
			// indistinguishable from an observation at the point a retry
			// policy reads it.
			name: "a server fault is not guessed at",
			raw:  `503: {"message":"upstream unavailable","type":"api_error","code":"503"}`,
			kind: FailureUnknown, code: "503", typ: "api_error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyPiTurnFailure(tc.raw)
			if got.Kind != tc.kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.kind)
			}
			if got.Code != tc.code {
				t.Errorf("Code = %q, want %q", got.Code, tc.code)
			}
			if got.Type != tc.typ {
				t.Errorf("Type = %q, want %q", got.Type, tc.typ)
			}
			// Raw is the contract every existing reader of this failure
			// depends on, including the ones that ignore Kind entirely.
			if got.Raw != tc.raw {
				t.Errorf("Raw = %q, want it byte-identical to the received message", got.Raw)
			}
		})
	}
}

func TestUnclassifiedTurnFailureKeepsItsText(t *testing.T) {
	f := UnclassifiedTurnFailure("codex turn.failed: something")
	if f.Kind != FailureUnknown || f.Raw != "codex turn.failed: something" {
		t.Fatalf("got %+v, want an unknown failure carrying its message", f)
	}
	if f.Code != "" || f.Type != "" {
		t.Errorf("got %+v, want no provider fields invented for a dialect with no envelope", f)
	}
}

// --- the classification as the runtime actually produces it ---

// jsonEscapedEnvelope embeds a measured envelope inside a pi event line,
// which is where the classifier meets it in production: the provider's JSON
// arrives as a string field inside pi's own JSON.
func jsonEscapedEnvelope(t *testing.T, raw string) string {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return string(encoded)
}

func drainRuntime(t *testing.T, dialect string, lines ...string) *SessionRuntime {
	t.Helper()
	r := newTestRuntime(dialect, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		for range r.Events() {
		}
	}()
	r.Start(ctx, strings.NewReader(strings.Join(lines, "\n")+"\n"), strings.NewReader(""))
	r.WaitDrained()
	return r
}

func TestSessionRuntime_PiRateLimitIsClassifiedFromTheStream(t *testing.T) {
	r := drainRuntime(t, "pi",
		`{"type":"session","version":3,"id":"019fc4ff-74ee-79d1-910b-734b253f6668"}`,
		`{"type":"message","id":"m1","message":{"role":"assistant","stopReason":"error","errorMessage":`+
			jsonEscapedEnvelope(t, measuredRateLimitEnvelope)+`}}`,
		`{"type":"agent_settled"}`)

	failure := r.LastFailure()
	if failure == nil {
		t.Fatal("a failed pi turn must record a typed failure")
	}
	if failure.Kind != FailureRateLimited || failure.Code != "429" {
		t.Fatalf("got %+v, want the provider's own 429 recognised", failure)
	}
	// Classified from the raw provider message, not from LastError - which
	// carries kernl's own "pi agent_error: " prefix in front of the payload.
	if failure.Raw != measuredRateLimitEnvelope {
		t.Errorf("Raw = %q, want the provider's message without kernl's prefix", failure.Raw)
	}
	if !strings.Contains(r.LastError(), "RateLimitError") {
		t.Errorf("LastError() = %q, want the existing message untouched", r.LastError())
	}
}

func TestSessionRuntime_PiNonProviderFailuresStayUnknown(t *testing.T) {
	// stopReason=aborted and stopReason=toolUse are kernl's own readings of
	// how a session ended, with no provider envelope behind them.
	for _, stopReason := range []string{"aborted", "toolUse"} {
		t.Run(stopReason, func(t *testing.T) {
			r := drainRuntime(t, "pi",
				`{"type":"message","id":"m1","message":{"role":"assistant","stopReason":"`+stopReason+`"}}`,
				`{"type":"agent_settled"}`)
			failure := r.LastFailure()
			if failure == nil || failure.Kind != FailureUnknown {
				t.Fatalf("got %+v, want an unknown failure", failure)
			}
			if failure.Raw != r.LastError() {
				t.Errorf("Raw = %q, want it to carry what was recorded (%q)", failure.Raw, r.LastError())
			}
		})
	}
}

func TestSessionRuntime_PiCleanTurnClearsTheFailure(t *testing.T) {
	r := drainRuntime(t, "pi",
		`{"type":"message","id":"m1","message":{"role":"assistant","stopReason":"error","errorMessage":`+
			jsonEscapedEnvelope(t, measuredRateLimitEnvelope)+`}}`,
		`{"type":"message","id":"m2","message":{"role":"assistant","stopReason":"stop"}}`,
		`{"type":"agent_settled"}`)

	if failure := r.LastFailure(); failure != nil {
		t.Fatalf("a turn that recovered must report no failure, got %+v", failure)
	}
}

// TestSessionRuntime_CodexFailureIsNotClassified is the limit of this
// classifier, asserted rather than left to be discovered. No rollout on disk
// has ever shown a provider fault in codex's envelope, so there is nothing
// measured to key on - and a parser written blind would answer with the same
// confidence as one written from evidence.
func TestSessionRuntime_CodexFailureIsNotClassified(t *testing.T) {
	r := drainRuntime(t, "codex",
		`{"type":"turn.failed","error":{"message":"429 rate limit","code":"429"}}`)

	failure := r.LastFailure()
	if failure == nil {
		t.Fatal("a failed codex turn must still record a failure")
	}
	if failure.Kind != FailureUnknown {
		t.Fatalf("got %+v, want unknown: codex's envelope has never been measured", failure)
	}
	if !strings.Contains(failure.Raw, "429 rate limit") {
		t.Errorf("Raw = %q, want the message preserved even unclassified", failure.Raw)
	}
}
