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

// Default TTS model fallback.
const (
	defaultTTSModel = "kokoro"
	// maxTTSInputLength caps the number of characters accepted for TTS synthesis.
	// OpenAI's limit is 4096; we use the same to prevent resource exhaustion.
	maxTTSInputLength = 4096
)

// Audio handles text-to-speech synthesis requests.
//
//	POST /v1/audio/speech
//
// Uses the router registry to select the appropriate TTS backend.
// Accepts the standard OpenAI-compatible TTS request body.
func Audio(rtr *router.Registry, hc backend.HealthChecker, logger *slog.Logger) http.HandlerFunc {
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
		if len(req.Input) > maxTTSInputLength {
			apierror.Write(w, apierror.InvalidParam("input", "input must be at most 4096 characters"))
			return
		}

		// Apply defaults.
		if strings.TrimSpace(req.Model) == "" {
			req.Model = defaultTTSModel
		}

		info, err := rtr.SelectHealthyBackend(router.CapTTS, req.Model, hc)
		if err != nil {
			logger.Error("no TTS backend available", "err", err)
			apierror.Write(w, apierror.BackendUnavailable(req.Model))
			return
		}

		// Apply voice default based on the selected backend.
		// Each TTS backend has its own set of known voices.
		if strings.TrimSpace(req.Voice) == "" {
			switch info.Name {
			case "kokoro":
				req.Voice = "af_bella"
			case "chatterbox":
				req.Voice = "chatterbox-default"
			default:
				req.Voice = "default"
			}
		}

		// Default omitted speed (JSON unmarshals to 0) before clamping.
		if req.Speed <= 0 {
			req.Speed = 1.0
		}
		// Clamp speed to OpenAI-compatible range [0.25, 4.0].
		if req.Speed < 0.25 {
			req.Speed = 0.25
		} else if req.Speed > 4.0 {
			req.Speed = 4.0
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
			apierror.Write(w, apierror.FromBackendError(info.Name, err))
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
