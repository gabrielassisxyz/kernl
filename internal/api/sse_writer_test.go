package api

import (
	"net/http/httptest"
	"testing"
)

// panicFlusher stands in for a spent http.response: flushing one dereferences a
// nil buffer and panics for real, which is how a background goroutine took the
// whole server down.
type panicFlusher struct{}

func (panicFlusher) Flush() { panic("flushed a response the handler already finished") }

// The engine's learned-candidate goroutine outlives the SSE handler and emits a
// state event when it is done. Once the handler returns, the ResponseWriter is
// spent — and a panic in a goroutine nobody recovers kills the process, not just
// the request. So a late write must be dropped, not attempted.
func TestSSEWriterGoesInertAfterTheHandlerReturns(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &sseEventWriter{w: rec, flusher: panicFlusher{}}

	w.close() // the handler returned

	n, err := w.Write([]byte("data: {\"event\":\"state\"}\n\n"))
	if err != nil {
		t.Errorf("a late write must be dropped, not fail: %v", err)
	}
	if n == 0 {
		t.Error("a dropped write must still report the bytes as consumed")
	}
	w.Flush() // must not reach the flusher, and must not panic

	if rec.Body.Len() != 0 {
		t.Errorf("wrote to a finished response: %q", rec.Body.String())
	}
}
