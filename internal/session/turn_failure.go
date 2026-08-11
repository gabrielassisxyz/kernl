package session

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FailureKind names how a dispatch's turn failed, in the only terms the
// evidence supports.
//
// The list is short on purpose. Every distinct pi failure envelope on this
// machine was inventoried before it was written, and exactly two of the four
// carry anything a program can key on: the provider's error arrives as JSON
// embedded in the message, with its own "code" and "type". The other two are
// bare English sentences ("Connection error.", a failed key resolution), and
// a kind recognisable only by matching a sentence is a kind that stops
// working the day the provider rewords it. Those stay FailureUnknown, which
// is what they are.
type FailureKind string

const (
	// FailureRateLimited is the provider refusing on rate, and the only kind
	// a later retry policy has any business retrying.
	FailureRateLimited FailureKind = "rate_limited"
	// FailureInvalidRequest is the provider refusing the request itself - a
	// model name that does not exist, a malformed call. Naming it is the
	// point: it will fail identically on every attempt, so a retry spends
	// real dispatches reproducing a typo.
	FailureInvalidRequest FailureKind = "invalid_request"
	// FailureUnknown is everything not structurally evidenced, including
	// every failure of every dialect whose envelope has not been measured.
	FailureUnknown FailureKind = "unknown"
)

// TurnFailure is how a turn failed, carried from the dialect parser that saw
// the envelope through to the attempt ledger.
//
// Raw is always the message as received. Code and Type are the provider's own
// fields when the payload had them, and they are recorded even when they did
// not change the Kind: they are what makes widening this vocabulary later a
// change to a switch rather than a second afternoon in the transcripts.
type TurnFailure struct {
	Kind FailureKind `json:"kind"`
	Code string      `json:"code,omitempty"`
	Type string      `json:"type,omitempty"`
	Raw  string      `json:"raw,omitempty"`
}

// UnclassifiedTurnFailure is the answer for a dialect whose failure envelope
// has not been measured, and for pi's own non-provider failures (an aborted
// session, a turn that ended mid-tool-call). It preserves what happened
// without claiming to know what kind of thing it was.
func UnclassifiedTurnFailure(raw string) *TurnFailure {
	return &TurnFailure{Kind: FailureUnknown, Raw: raw}
}

// providerErrorPayload is the JSON the provider embeds in pi's errorMessage.
// Code is decoded loosely because a provider that reports it as a number
// rather than a string is reporting the same fact.
type providerErrorPayload struct {
	Code any    `json:"code"`
	Type string `json:"type"`
}

// ClassifyPiTurnFailure reads one pi errorMessage and names what happened.
//
// The measured envelope is a status prefix followed by the provider's own
// JSON, e.g.
//
//	429: {"message":"litellm.RateLimitError: …","type":"throttling_error","code":"429"}
//
// The payload's code is the test, not the prefix and not the prose: this is
// the same choice backend.CloseRefusedError already makes one layer down,
// where br's structured refusal is classified rather than its message parsed.
// A message with no payload, or one whose payload will not decode, is
// FailureUnknown with its text intact - never an error, because the caller is
// already reporting a failure and losing its description to a second one
// helps nobody.
func ClassifyPiTurnFailure(raw string) *TurnFailure {
	failure := &TurnFailure{Kind: FailureUnknown, Raw: raw}

	brace := strings.IndexByte(raw, '{')
	if brace < 0 {
		return failure
	}
	var payload providerErrorPayload
	if err := json.Unmarshal([]byte(raw[brace:]), &payload); err != nil {
		return failure
	}

	failure.Type = payload.Type
	failure.Code = normalizeProviderCode(payload.Code)
	failure.Kind = kindForStatusCode(failure.Code)
	return failure
}

// kindForStatusCode maps an HTTP status the provider reported to a kind.
// 5xx is deliberately not named: nothing measured has produced one, and a
// guess about a server fault would be indistinguishable, at the point a retry
// policy reads it, from something that was actually observed.
func kindForStatusCode(code string) FailureKind {
	status, err := strconv.Atoi(code)
	if err != nil {
		return FailureUnknown
	}
	switch {
	case status == 429:
		return FailureRateLimited
	case status >= 400 && status < 500:
		return FailureInvalidRequest
	default:
		return FailureUnknown
	}
}

func normalizeProviderCode(code any) string {
	switch v := code.(type) {
	case string:
		return v
	case float64:
		// encoding/json decodes every JSON number into float64, and a status
		// code is an integer in every shape a provider sends it.
		return strconv.Itoa(int(v))
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}
