// Package decision provides clients for System One decision engines
// (TypeSafe Jev, Laya).
//
// Unlike chat models, decision engines answer typed questions over a state and
// return structured answers with probabilities and confidence. Both Jev and
// Laya speak the same wire protocol:
//
//	POST {base}/v1/systemone
//	{"state": ..., "model": ..., "questions": {...}}
//
// so one client covers both; only the base URL and API key differ.
package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout bounds a single decision request. Laya can spend seconds
// loading a checkpoint on first use; Jev answers in 70-500 ms.
const DefaultTimeout = 30 * time.Second

// Client talks to one System One decision engine.
type Client struct {
	name    string
	baseURL string
	apiKey  string
	models  []string
	client  *http.Client
}

// New creates a decision client. models lists the engine's model IDs for
// discovery and request routing (Jev: "jev-latest"; Laya: "laya", ...).
func New(name, baseURL, apiKey string, models ...string) *Client {
	return &Client{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		models:  models,
		client:  &http.Client{Timeout: DefaultTimeout},
	}
}

func (c *Client) Name() string { return c.name }

// Models returns the advertised model IDs for this engine.
func (c *Client) Models() []string { return c.models }

// Result is an upstream decision response relayed verbatim.
type Result struct {
	Status int
	Body   []byte
}

// Do sends a raw SystemOne request body and returns the upstream response.
// Transport failures return an error; non-2xx statuses are relayed as-is so
// callers keep the engine's own error shape.
func (c *Client) Do(ctx context.Context, body []byte) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decision %s: create request: %w", c.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("decision %s: %w", c.name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decision %s: read response: %w", c.name, err)
	}
	return &Result{Status: resp.StatusCode, Body: respBody}, nil
}

// ErrAbstained is returned by Pick when the engine declines to choose
// (min_confidence gate), so callers can distinguish "unsure" from "failed".
var ErrAbstained = errors.New("decision engine abstained")

type question struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria,omitempty"`
}

type predictRequest struct {
	State         string              `json:"state"`
	Model         string              `json:"model,omitempty"`
	MinConfidence float64             `json:"min_confidence,omitempty"`
	Questions     map[string]question `json:"questions"`
}

type predictResponse struct {
	Answers map[string]struct {
		Type             string  `json:"type"`
		Choice           string  `json:"choice"`
		Confidence       float64 `json:"confidence"`
		AnswerConfidence float64 `json:"answer_confidence"`
		Abstention       string  `json:"abstention"`
	} `json:"answers"`
}

// Pick asks a single choice question ("tier") over the state and returns the
// chosen label with its confidence. model pins an engine checkpoint when set.
// minConfidence > 0 forwards the engine's abstention gate and also enforces it
// locally; an abstention or a below-threshold answer returns ErrAbstained.
func (c *Client) Pick(ctx context.Context, state, instructions, model string, criteria map[string]string, minConfidence float64) (string, float64, error) {
	if len(criteria) == 0 {
		return "", 0, fmt.Errorf("decision %s: no criteria given", c.name)
	}
	if instructions == "" {
		instructions = "Which option should handle this request?"
	}

	body, err := json.Marshal(predictRequest{
		State:         state,
		Model:         model,
		MinConfidence: minConfidence,
		Questions: map[string]question{
			"tier": {Type: "choice", Instructions: instructions, Criteria: criteria},
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("decision %s: marshal pick request: %w", c.name, err)
	}

	res, err := c.Do(ctx, body)
	if err != nil {
		return "", 0, err
	}
	if res.Status < 200 || res.Status >= 300 {
		return "", 0, fmt.Errorf("decision %s: status %d: %s", c.name, res.Status, truncate(res.Body))
	}

	var decoded predictResponse
	if err := json.Unmarshal(res.Body, &decoded); err != nil {
		return "", 0, fmt.Errorf("decision %s: decode response: %w", c.name, err)
	}
	answer, ok := decoded.Answers["tier"]
	if !ok || answer.Choice == "" || answer.Abstention == "abstained" {
		if answer.Abstention == "abstained" {
			return "", answer.Confidence, fmt.Errorf("%w: %s", ErrAbstained, c.name)
		}
		return "", 0, fmt.Errorf("decision %s: response has no tier choice", c.name)
	}
	effective := answer.Confidence
	if answer.AnswerConfidence > 0 {
		effective = answer.AnswerConfidence
	}
	if minConfidence > 0 && effective < minConfidence {
		return "", effective, fmt.Errorf("%w: %s confidence %.2f below %.2f", ErrAbstained, c.name, effective, minConfidence)
	}
	return answer.Choice, effective, nil
}

func truncate(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}

// Set holds the configured decision engines in configuration order.
type Set struct {
	clients []*Client
	byName  map[string]*Client
}

// NewSet creates a Set from the given clients.
func NewSet(clients ...*Client) *Set {
	s := &Set{byName: make(map[string]*Client)}
	for _, c := range clients {
		if c == nil {
			continue
		}
		s.clients = append(s.clients, c)
		s.byName[c.Name()] = c
	}
	return s
}

// All returns the clients in configuration order.
func (s *Set) All() []*Client {
	if s == nil {
		return nil
	}
	return s.clients
}

// Get returns the client with the given name.
func (s *Set) Get(name string) (*Client, bool) {
	if s == nil {
		return nil, false
	}
	c, ok := s.byName[name]
	return c, ok
}

// First returns the first configured client (local-first when Laya is listed first).
func (s *Set) First() (*Client, bool) {
	if s == nil || len(s.clients) == 0 {
		return nil, false
	}
	return s.clients[0], true
}

// Len returns the number of configured engines.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.clients)
}
