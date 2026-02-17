package handler

import (
	"io"
	"log/slog"
)

// discardLogger returns a logger that writes to /dev/null.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
