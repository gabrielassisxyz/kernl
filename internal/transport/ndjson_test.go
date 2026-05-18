package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type chunkReader struct {
	chunks [][]byte
	idx    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.idx])
	r.idx++
	return n, nil
}

func TestParseNDJSON_CompleteLines(t *testing.T) {
	input := `{"type":"message","content":"hello"}
{"type":"result","data":"world"}
{"type":"done"}` + "\n"

	ch := ParseNDJSON(context.Background(), strings.NewReader(input))
	var results []ParsedLine
	for line := range ch {
		results = append(results, line)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(results))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("line %d should not have error: %v", i, r.Err)
		}
		if len(r.Data) == 0 {
			t.Errorf("line %d should have data", i)
		}
	}

	var msg map[string]string
	if err := json.Unmarshal(results[0].Data, &msg); err != nil {
		t.Errorf("unmarshal line 0: %v", err)
	}
	if msg["type"] != "message" {
		t.Errorf("expected type=message, got %s", msg["type"])
	}
}

func TestParseNDJSON_ChunkedBoundaries(t *testing.T) {
	reader := &chunkReader{
		chunks: [][]byte{
			[]byte(`{"a":`),
			[]byte(`1}` + "\n" + `{"a":2}` + "\n"),
		},
	}

	ch := ParseNDJSON(context.Background(), reader)
	var results []ParsedLine
	for line := range ch {
		results = append(results, line)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 lines from chunked data, got %d", len(results))
	}

	var v map[string]int
	if err := json.Unmarshal(results[0].Data, &v); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if v["a"] != 1 {
		t.Errorf("expected a=1, got %d", v["a"])
	}
	if err := json.Unmarshal(results[1].Data, &v); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if v["a"] != 2 {
		t.Errorf("expected a=2, got %d", v["a"])
	}
}

func TestParseNDJSON_TrailingPartialFlush(t *testing.T) {
	input := `{"a":1}`
	ch := ParseNDJSON(context.Background(), strings.NewReader(input))
	var results []ParsedLine
	for line := range ch {
		results = append(results, line)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 line from partial trailing, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("should not have error: %v", results[0].Err)
	}

	var v map[string]int
	if err := json.Unmarshal(results[0].Data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v["a"] != 1 {
		t.Errorf("expected a=1, got %d", v["a"])
	}
}

func TestParseNDJSON_TrailingPartialInvalidJSON(t *testing.T) {
	input := `{"broken`
	ch := ParseNDJSON(context.Background(), strings.NewReader(input))
	var results []ParsedLine
	for line := range ch {
		results = append(results, line)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 line for invalid trailing, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("invalid JSON in trailing partial must surface as parse error")
	}
	if results[0].Line != `{"broken` {
		t.Errorf("expected raw line to be preserved, got %q", results[0].Line)
	}
}

func TestParseNDJSON_SkipEmptyLines(t *testing.T) {
	input := `{"a":1}` + "\n\n\n" + `{"a":2}` + "\n"
	ch := ParseNDJSON(context.Background(), strings.NewReader(input))
	var results []ParsedLine
	for line := range ch {
		results = append(results, line)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 non-empty lines, got %d", len(results))
	}
}

func TestParseNDJSON_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := ParseNDJSON(ctx, strings.NewReader(`{"type":"test"}`+"\n"))

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after context cancellation")
	}
}

func TestParseNDJSON_ContextCancellationMidStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	slowReader := &slowChunkReader{
		chunks: [][]byte{
			[]byte(`{"a":1}` + "\n"),
			[]byte(`{"a":2}` + "\n"),
		},
		delay: 200 * time.Millisecond,
	}

	ch := ParseNDJSON(ctx, slowReader)
	var count int
	for range ch {
		count++
	}

	if count > 2 {
		t.Errorf("expected at most 2 lines before cancellation, got %d", count)
	}
}

func TestParseNDJSON_ReadError(t *testing.T) {
	reader := &errorReader{err: errors.New("read failure")}
	ch := ParseNDJSON(context.Background(), reader)

	var results []ParsedLine
	for line := range ch {
		results = append(results, line)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 error result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected read error to be surfaced")
	}
}

func TestParseNDJSON_MixedValidAndInvalidLines(t *testing.T) {
	input := `{"valid":true}` + "\n" + `not json` + "\n" + `{"also_valid":1}` + "\n"
	ch := ParseNDJSON(context.Background(), strings.NewReader(input))

	var results []ParsedLine
	for line := range ch {
		results = append(results, line)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("line 0 should be valid: %v", results[0].Err)
	}
	if results[1].Err == nil {
		t.Error("line 1 should have parse error")
	}
	if results[2].Err != nil {
		t.Errorf("line 2 should be valid: %v", results[2].Err)
	}
}

type slowChunkReader struct {
	chunks [][]byte
	idx    int
	delay  time.Duration
}

func (r *slowChunkReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	n := copy(p, r.chunks[r.idx])
	r.idx++
	return n, nil
}

type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}