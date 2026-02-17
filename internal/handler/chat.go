package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/menez/inferencia/internal/apierror"
	"github.com/menez/inferencia/internal/backend"
)

// ChatCompletions handles chat completion requests, supporting both
// standard JSON responses and streaming SSE responses.
//
//	POST /v1/chat/completions
func ChatCompletions(reg *backend.Registry, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req backend.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierror.Write(w, apierror.InvalidRequest("Invalid JSON in request body: "+err.Error()))
			return
		}

		if len(req.Messages) == 0 {
			apierror.Write(w, apierror.InvalidParam("messages", "messages is required and must not be empty"))
			return
		}

		b, err := reg.Primary()
		if err != nil {
			apierror.Write(w, apierror.BackendUnavailable("default"))
			return
		}

		if req.Stream {
			handleStream(w, r, b, req, logger)
			return
		}

		handleJSON(w, r, b, req, logger)
	}
}

// handleJSON processes a non-streaming chat completion request.
func handleJSON(w http.ResponseWriter, r *http.Request, b backend.Backend, req backend.ChatRequest, logger *slog.Logger) {
	resp, err := b.ChatCompletion(r.Context(), req)
	if err != nil {
		logger.Error("chat completion failed", "backend", b.Name(), "err", err)
		apierror.Write(w, apierror.BackendUnavailable(b.Name()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.Error("failed to encode chat response", "err", err)
	}
}

// handleStream processes a streaming chat completion request using SSE.
func handleStream(w http.ResponseWriter, r *http.Request, b backend.Backend, req backend.ChatRequest, logger *slog.Logger) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		apierror.Write(w, apierror.Internal("Streaming not supported by this server."))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering if behind reverse proxy.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	send := func(data []byte) error {
		// Check if client disconnected.
		if r.Context().Err() != nil {
			return r.Context().Err()
		}

		if string(data) == "[DONE]" {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return nil
		}

		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return nil
	}

	if err := b.ChatCompletionStream(r.Context(), req, send); err != nil {
		// If streaming already started, we can't send an error response.
		// Log it and let the client handle the broken stream.
		logger.Error("stream error", "backend", b.Name(), "err", err)
	}
}
