package chat

import "io"

// ChatEventWriter is the interface implemented by SSE response writers.
type ChatEventWriter interface {
	io.Writer
	Flush()
}
