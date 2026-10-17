// Package catalog holds model prices and the one function that turns token
// usage into dollars. The default catalog is embedded; a file can replace it.
package catalog

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/menezmethod/inferencia/internal/ledger"
)

//go:embed catalog.yaml
var embedded []byte

// ErrUnknownModel means the catalog has no entry for a model. Never price it as $0.
var ErrUnknownModel = errors.New("model not in catalog")

// ErrMissingPrice means the model is known but lacks a price the usage needs.
var ErrMissingPrice = errors.New("price missing")

// ErrBadUsage means token counts that no real call can produce.
var ErrBadUsage = errors.New("invalid usage")

// Price is USD per million tokens. A nil field is unknown, not zero.
type Price struct {
	Input        *float64 `yaml:"input"`
	Output       *float64 `yaml:"output"`
	CacheRead    *float64 `yaml:"cache_read"`
	CacheWrite   *float64 `yaml:"cache_write"`
	CacheWrite1h *float64 `yaml:"cache_write_1h"`
}

func (p Price) all() []*float64 {
	return []*float64{p.Input, p.Output, p.CacheRead, p.CacheWrite, p.CacheWrite1h}
}

// LongContext is a higher price for requests whose prompt (input, cache reads
// and cache writes) is over Above tokens, priced per request.
type LongContext struct {
	Above int64 `yaml:"above"`
	Price Price `yaml:"price"`
}

// Model is one catalog entry.
type Model struct {
	ID               string       `yaml:"id"`
	Aliases          []string     `yaml:"aliases"`
	Provider         string       `yaml:"provider"`
	Tier             string       `yaml:"tier"`
	DataPolicy       string       `yaml:"data_policy"`
	DataPolicySource string       `yaml:"data_policy_source"`
	Price            Price        `yaml:"price"`
	LongContext      *LongContext `yaml:"long_context"`
	Source           string       `yaml:"source"`
}

// Plan is a flat subscription with its list prices per tier (USD per month).
type Plan struct {
	ID        string             `yaml:"id"`
	Upstreams []string           `yaml:"upstreams"`
	Harness   string             `yaml:"harness"`
	Tiers     map[string]float64 `yaml:"tiers"`
	Source    string             `yaml:"source"`
}

// Catalog is the parsed catalog file.
type Catalog struct {
	AsOf    string            `yaml:"as_of"`
	Sources map[string]string `yaml:"sources"`
	Models  []Model           `yaml:"models"`
	Plans   []Plan            `yaml:"plans"`

	byName map[string]*Model
}

// Default returns the embedded catalog.
func Default() (*Catalog, error) { return Parse(embedded) }

// Load reads a catalog file.
func Load(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

var tiers = map[string]bool{"frontier": true, "strong": true, "fast": true, "local": true}
var policies = map[string]bool{"zdr": true, "no-train": true, "trains": true, "unknown": true}

// Parse decodes and validates a catalog. Unknown fields, a model without an
// https source, an unknown tier or data policy, a duplicate name, or a model
// without input and output prices are all errors.
func Parse(b []byte) (*Catalog, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var c Catalog
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if c.AsOf == "" {
		return nil, errors.New("catalog: as_of is required")
	}
	c.byName = map[string]*Model{}
	for i := range c.Models {
		m := &c.Models[i]
		switch {
		case m.ID == "":
			return nil, fmt.Errorf("catalog: model %d has no id", i)
		case !strings.HasPrefix(m.Source, "https://"):
			return nil, fmt.Errorf("catalog: %s: source must be an https URL", m.ID)
		case !tiers[m.Tier]:
			return nil, fmt.Errorf("catalog: %s: unknown tier %q", m.ID, m.Tier)
		case !policies[m.DataPolicy]:
			return nil, fmt.Errorf("catalog: %s: unknown data_policy %q", m.ID, m.DataPolicy)
		case m.Price.Input == nil || m.Price.Output == nil:
			return nil, fmt.Errorf("catalog: %s: input and output prices are required", m.ID)
		case !validPrice(m.Price):
			return nil, fmt.Errorf("catalog: %s: prices must be finite and not negative", m.ID)
		}
		if lc := m.LongContext; lc != nil {
			if lc.Above <= 0 || lc.Price.Input == nil || lc.Price.Output == nil || !validPrice(lc.Price) {
				return nil, fmt.Errorf("catalog: %s: long_context needs above > 0 and valid input and output prices", m.ID)
			}
		}
		for _, name := range append([]string{m.ID}, m.Aliases...) {
			if _, dup := c.byName[name]; dup {
				return nil, fmt.Errorf("catalog: %s: name %q appears twice", m.ID, name)
			}
			c.byName[name] = m
		}
	}
	for _, p := range c.Plans {
		if !strings.HasPrefix(p.Source, "https://") {
			return nil, fmt.Errorf("catalog: plan %s: source must be an https URL", p.ID)
		}
		if len(p.Tiers) == 0 {
			return nil, fmt.Errorf("catalog: plan %s: needs at least one tier", p.ID)
		}
		for name, usd := range p.Tiers {
			if !(usd > 0) || math.IsInf(usd, 0) {
				return nil, fmt.Errorf("catalog: plan %s: tier %s must cost more than $0", p.ID, name)
			}
		}
	}
	return &c, nil
}

// Lookup finds a model by id or alias.
func (c *Catalog) Lookup(name string) (*Model, error) {
	if m, ok := c.byName[name]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownModel, name)
}

func validPrice(p Price) bool {
	for _, v := range p.all() {
		if v != nil && (*v < 0 || math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return false
		}
	}
	return true
}

// PriceFor returns the price that applies to u: the long-context price when
// the average prompt per call is over the threshold. Codex records are whole
// sessions, so for them the average is an approximation.
func (m *Model) PriceFor(u ledger.Usage) Price {
	if lc := m.LongContext; lc != nil {
		prompt := (u.Input + u.CacheRead + u.CacheWrite) / int64(max(u.Calls, 1))
		if prompt > lc.Above {
			return lc.Price
		}
	}
	return m.Price
}

// Cost prices one usage record at a model's list prices. It is the only
// price function: (uncached input x input + cache reads x cache read + 5-minute
// writes x cache write + 1-hour writes x 1-hour write + output x output) / 1e6.
// A token count that needs a missing price is an error, never $0.
func (m *Model) Cost(u ledger.Usage) (float64, error) {
	if u.Input < 0 || u.CacheRead < 0 || u.CacheWrite < 0 || u.CacheWrite1h < 0 || u.Output < 0 || u.CacheWrite1h > u.CacheWrite {
		return 0, fmt.Errorf("%w: negative tokens or 1-hour writes above all writes", ErrBadUsage)
	}
	p := m.PriceFor(u)
	write5m := u.CacheWrite - u.CacheWrite1h
	parts := []struct {
		tokens int64
		price  *float64
		name   string
	}{
		{u.Input, p.Input, "input"},
		{u.CacheRead, p.CacheRead, "cache_read"},
		{write5m, p.CacheWrite, "cache_write"},
		{u.CacheWrite1h, p.CacheWrite1h, "cache_write_1h"},
		{u.Output, p.Output, "output"},
	}
	var usd float64
	for _, part := range parts {
		if part.tokens == 0 {
			continue
		}
		if part.price == nil {
			return 0, fmt.Errorf("%w: %s has %d %s tokens and no %s price", ErrMissingPrice, m.ID, part.tokens, part.name, part.name)
		}
		usd += float64(part.tokens) * *part.price / 1e6
	}
	return usd, nil
}

// Cost prices a usage record at the list price of the model it ran on.
func (c *Catalog) Cost(u ledger.Usage) (float64, error) {
	m, err := c.Lookup(u.Model)
	if err != nil {
		return 0, err
	}
	return m.Cost(u)
}

// PlanFor returns the plan that covers a usage record, if any.
func (c *Catalog) PlanFor(u ledger.Usage) (*Plan, bool) {
	for i := range c.Plans {
		p := &c.Plans[i]
		if p.Harness != u.Harness {
			continue
		}
		for _, up := range p.Upstreams {
			if up == u.Upstream {
				return p, true
			}
		}
	}
	return nil, false
}
