package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/menezmethod/inferencia/internal/apierror"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/middleware"
)

const defaultChatModel = "qwen3.6:35b-a3b-coding-bf16"

// ChatCompletions handles chat completion requests, supporting both
// standard JSON responses and streaming SSE responses.
//
//	POST /v1/chat/completions
func ChatCompletions(reg *backend.Registry, hc backend.HealthChecker, logger *slog.Logger) http.HandlerFunc {
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
		if strings.TrimSpace(req.Model) == "" {
			req.Model = defaultChatModel
		}

		b, err := reg.PrimaryHealthy(hc)
		if err != nil {
			apierror.Write(w, backendSelectError(reg, err))
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
		apierror.Write(w, apierror.FromBackendError(b.Name(), err))
		return
	}

	if resp.Usage != nil {
		middleware.TokensTotal.WithLabelValues(resp.Model, "prompt").Add(float64(resp.Usage.PromptTokens))
		middleware.TokensTotal.WithLabelValues(resp.Model, "completion").Add(float64(resp.Usage.CompletionTokens))
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
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var mu sync.Mutex
	send := func(data []byte) error {
		if r.Context().Err() != nil {
			return r.Context().Err()
		}

		mu.Lock()
		defer mu.Unlock()

		if string(data) == "[DONE]" {
			_, err := fmt.Fprintf(w, "data: [DONE]\n\n")
			if err != nil {
				return fmt.Errorf("client disconnected: %w", err)
			}
			flusher.Flush()
			return nil
		}

		_, err := fmt.Fprintf(w, "data: %s\n\n", data)
		if err != nil {
			return fmt.Errorf("client disconnected: %w", err)
		}
		flusher.Flush()
		return nil
	}

	if err := b.ChatCompletionStream(r.Context(), req, send); err != nil {
		logger.Error("stream error", "backend", b.Name(), "err", err)
	}
}

func backendSelectError(reg *backend.Registry, err error) *apierror.Error {
	name := reg.PrimaryName()
	if name == "" {
		name = "default"
	}
	if errors.Is(err, backend.ErrNoHealthyBackend) || errors.Is(err, backend.ErrBackendNotFound) {
		return apierror.BackendUnavailable(name)
	}
	return apierror.BackendUnavailable(name)
}
