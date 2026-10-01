package backend

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// ErrEmbeddingsUnsupported is returned by backends with no embeddings route.
var ErrEmbeddingsUnsupported = errors.New("embeddings not supported by this backend")

// relaySSE copies an upstream SSE body to send, forwarding data frames and
// stopping at [DONE]. Lines without the "data: " prefix are skipped.
// A missing [DONE] terminator is tolerated: the stream simply ends.
func relaySSE(body io.Reader, send StreamFunc) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			if err := send([]byte(data)); err != nil {
				return err
			}
			break
		}
		if err := send([]byte(data)); err != nil {
			return err
		}
	}
	return scanner.Err()
}
