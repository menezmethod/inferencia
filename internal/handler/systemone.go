package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/menezmethod/inferencia/internal/apierror"
	"github.com/menezmethod/inferencia/internal/decision"
)

// maxSystemOneBody caps decision request bodies (states are text/JSON, answers are small).
const maxSystemOneBody = 1 << 20

// SystemOne handles System One decision requests and relays them to a
// configured decision engine (TypeSafe Jev, Laya).
//
//	POST /v1/systemone
//
// The engine is selected from the request's "model": a "backend/model" prefix
// picks that engine (the prefix is stripped before forwarding), a model ID
// listed by an engine picks that engine, otherwise the first configured engine
// is used (local-first when Laya is listed first). The upstream response is
// relayed verbatim so callers keep Jev's or Laya's own answer shape.
func SystemOne(decisions *decision.Set, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if decisions.Len() == 0 {
			apierror.Write(w, apierror.BackendUnavailable("decisions"))
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxSystemOneBody)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			apierror.Write(w, apierror.InvalidRequest("Invalid request body: "+err.Error()))
			return
		}

		client, body := selectDecision(decisions, raw)

		res, err := client.Do(r.Context(), body)
		if err != nil {
			logger.Error("decision request failed", "backend", client.Name(), "err", err)
			apierror.Write(w, apierror.FromBackendError(client.Name(), err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Decision-Backend", client.Name())
		w.WriteHeader(res.Status)
		if _, err := w.Write(res.Body); err != nil {
			logger.Error("failed to write decision response", "err", err)
		}
	}
}

// selectDecision picks the engine for a request and returns the body to send.
// Only a matched "backend/" prefix rewrites the model field; other fields and
// the raw body are preserved.
func selectDecision(decisions *decision.Set, raw []byte) (*decision.Client, []byte) {
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(raw, &probe); err == nil && probe.Model != "" {
		if name, rest, ok := strings.Cut(probe.Model, "/"); ok {
			if client, found := decisions.Get(name); found {
				return client, rewriteModel(raw, rest)
			}
		}
		for _, client := range decisions.All() {
			for _, m := range client.Models() {
				if m == probe.Model {
					return client, raw
				}
			}
		}
	}
	client, _ := decisions.First()
	return client, raw
}

// rewriteModel replaces the model field in a request body, preserving all other fields.
func rewriteModel(raw []byte, model string) []byte {
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return raw
	}
	fields["model"] = model
	rewritten, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return rewritten
}
