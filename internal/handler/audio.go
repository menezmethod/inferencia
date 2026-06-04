package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/menezmethod/inferencia/internal/apierror"
	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/middleware"
	"github.com/menezmethod/inferencia/internal/router"
)

// Default TTS model and voice fallbacks.
const (
	defaultTTSModel = "kokoro"
	defaultTTSVoice = "af_bella"
)

// Audio handles text-to-speech synthesis requests.
//
//	POST /v1/audio/speech
//
// Uses the router registry to select the appropriate TTS backend.
// Accepts the standard OpenAI-compatible TTS request body.
func Audio(rtr *router.Registry, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req backend.TTSRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apierror.Write(w, apierror.InvalidRequest("Invalid JSON in request body: "+err.Error()))
			return
		}

		if strings.TrimSpace(req.Input) == "" {
			apierror.Write(w, apierror.InvalidParam("input", "input is required and must not be empty"))
			return
		}

		// Apply defaults.
		if strings.TrimSpace(req.Model) == "" {
			req.Model = defaultTTSModel
		}
		if strings.TrimSpace(req.Voice) == "" {
			req.Voice = defaultTTSVoice
		}

		// Select the TTS backend.
		info, err := rtr.SelectBackend(router.CapTTS, req.Model)
		if err != nil {
			logger.Error("no TTS backend available", "err", err)
			apierror.Write(w, apierror.BackendUnavailable("tts"))
			return
		}

		middleware.RoutingDecisionsTotal.WithLabelValues("tts", info.Name).Inc()

		if info.TTSBackend == nil {
			logger.Error("selected backend has no TTS backend", "name", info.Name)
			apierror.Write(w, apierror.BackendUnavailable(info.Name))
			return
		}

		start := time.Now()
		resp, err := info.TTSBackend.Synthesize(r.Context(), req)
		elapsed := time.Since(start)

		if err != nil {
			middleware.TTSRequestsTotal.WithLabelValues(info.Name, "error").Inc()
			logger.Error("tts synthesis failed", "backend", info.Name, "err", err)
			apierror.Write(w, apierror.BackendUnavailable(info.Name))
			return
		}

		middleware.TTSRequestsTotal.WithLabelValues(info.Name, "success").Inc()
		middleware.TTSRequestDuration.WithLabelValues(info.Name).Observe(elapsed.Seconds())
		middleware.TTSCharactersTotal.WithLabelValues(info.Name).Add(float64(len(req.Input)))

		// Determine Content-Type from response or the requested format.
		contentType := resp.Format
		if contentType == "" {
			contentType = mimeTypeFromFormat(req.ResponseFormat)
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(resp.Audio)))
		if _, err := w.Write(resp.Audio); err != nil {
			logger.Error("failed to write audio response", "err", err)
		}
	}
}

// mimeTypeFromFormat maps the OpenAI TTS response_format to a MIME type.
func mimeTypeFromFormat(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/opus"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	default:
		return "audio/wav"
	}
}
