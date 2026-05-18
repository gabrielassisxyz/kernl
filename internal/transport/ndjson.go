package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
)

type ParsedLine struct {
	Data json.RawMessage
	Line string
	Err  error
}

// ParseNDJSON reads newline-delimited JSON from reader, yielding each
// parsed line on the returned channel. It reassembles lines split across
// read chunks, skips empty lines, flushes trailing partial lines on
// EOF, and respects context cancellation at every line boundary.
//
// Invalid JSON in any line (including a trailing partial) surfaces as
// a ParsedLine with Err set — never silently swallowed.
//
// Usage: for line := range ParseNDJSON(ctx, resp.Body) { ... }
func ParseNDJSON(ctx context.Context, reader io.Reader) <-chan ParsedLine {
	ch := make(chan ParsedLine, 5000)

	go func() {
		defer close(ch)

		buf := make([]byte, 4096)
		var pending []byte

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, readErr := reader.Read(buf)
			if n > 0 {
				pending = append(pending, buf[:n]...)
				for {
					idx := bytes.IndexByte(pending, '\n')
					if idx < 0 {
						break
					}

					line := bytes.TrimSpace(pending[:idx])
					pending = pending[idx+1:]

					if len(line) == 0 {
						continue
					}

					select {
					case <-ctx.Done():
						return
					default:
					}

					var raw json.RawMessage
					if err := json.Unmarshal(line, &raw); err != nil {
						ch <- ParsedLine{Line: string(line), Err: err}
					} else {
						ch <- ParsedLine{Data: raw, Line: string(line)}
					}
				}
			}

			if readErr != nil {
				if readErr == io.EOF {
					remaining := bytes.TrimSpace(pending)
					if len(remaining) > 0 {
						var raw json.RawMessage
						if err := json.Unmarshal(remaining, &raw); err != nil {
							ch <- ParsedLine{Line: string(remaining), Err: err}
						} else {
							ch <- ParsedLine{Data: raw, Line: string(remaining)}
						}
					}
				} else {
					ch <- ParsedLine{Err: readErr}
				}
				return
			}
		}
	}()

	return ch
}