// Package watchdog runs a background goroutine that periodically health-checks
// all registered backends and updates Prometheus gauges + readiness state.
package watchdog

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/menezmethod/inferencia/internal/backend"
	"github.com/menezmethod/inferencia/internal/middleware"
	"github.com/menezmethod/inferencia/internal/router"
)

// Config controls the watchdog's behaviour.
type Config struct {
	Interval       time.Duration // how often to probe (default 30s)
	FailThreshold  int           // consecutive failures before DEGRADED (default 3)
	RequestTimeout time.Duration // per-probe HTTP timeout (default 5s)
}

// DefaultConfig returns production-grade defaults.
func DefaultConfig() Config {
	return Config{
		Interval:       30 * time.Second,
		FailThreshold:  3,
		RequestTimeout: 5 * time.Second,
	}
}

// backendState tracks consecutive failures for one backend.
type backendState struct {
	failures int
	healthy  bool
}

// Watchdog periodically probes chat/embed and TTS backends,
// updates Prometheus gauges, and marks backends degraded after
// consecutive failures.
type Watchdog struct {
	cfg    Config
	reg    *backend.Registry
	ttsReg *router.Registry
	logger *slog.Logger

	mu     sync.RWMutex
	states map[string]*backendState

	cancel context.CancelFunc
}

// New creates a Watchdog but does not start it.
func New(cfg Config, reg *backend.Registry, ttsReg *router.Registry, logger *slog.Logger) *Watchdog {
	return &Watchdog{
		cfg:    cfg,
		reg:    reg,
		ttsReg: ttsReg,
		logger: logger,
		states: make(map[string]*backendState),
	}
}

// Start launches the background health-check loop. Call Stop to shut it down.
func (w *Watchdog) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	w.probe(ctx)

	go func() {
		ticker := time.NewTicker(w.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.probe(ctx)
			}
		}
	}()

	w.logger.Info("watchdog started",
		"interval", w.cfg.Interval,
		"fail_threshold", w.cfg.FailThreshold,
	)
}

// Stop cancels the background loop.
func (w *Watchdog) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

// IsHealthy returns whether a named backend is currently healthy.
func (w *Watchdog) IsHealthy(name string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if s, ok := w.states[name]; ok {
		return s.healthy
	}
	return false
}

// probe runs one health-check cycle across all backends.
func (w *Watchdog) probe(parent context.Context) {
	// Chat/embed backends.
	for _, b := range w.reg.All() {
		w.checkBackend(parent, b.Name(), func(ctx context.Context) error {
			return b.Health(ctx)
		})
	}

	// TTS backends.
	if w.ttsReg != nil {
		for _, info := range w.ttsReg.All() {
			if info.TTSBackend != nil {
				w.checkBackend(parent, info.Name, func(ctx context.Context) error {
					return info.TTSBackend.Health(ctx)
				})
			}
		}
	}
}

// checkBackend probes a single backend and updates state + Prometheus gauge.
func (w *Watchdog) checkBackend(parent context.Context, name string, healthFn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(parent, w.cfg.RequestTimeout)
	defer cancel()

	err := healthFn(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()

	st, ok := w.states[name]
	if !ok {
		st = &backendState{healthy: true}
		w.states[name] = st
	}

	if err != nil {
		st.failures++
		if st.failures >= w.cfg.FailThreshold && st.healthy {
			st.healthy = false
			middleware.BackendHealth.WithLabelValues(name).Set(0)
			w.logger.Warn("backend marked DEGRADED",
				"backend", name,
				"consecutive_failures", st.failures,
			)
		} else if !st.healthy {
			middleware.BackendHealth.WithLabelValues(name).Set(0)
		} else {
			middleware.BackendHealth.WithLabelValues(name).Set(1)
			w.logger.Debug("backend probe failed (below threshold)",
				"backend", name,
				"err", err,
				"failures", st.failures,
			)
		}
	} else {
		wasDown := !st.healthy
		st.failures = 0
		st.healthy = true
		middleware.BackendHealth.WithLabelValues(name).Set(1)
		if wasDown {
			w.logger.Info("backend recovered", "backend", name)
		}
	}
}
