package backend

import (
	"context"
	"errors"
	"fmt"
)

// HealthChecker reports whether a backend should receive new inference traffic.
// A nil HealthChecker means all backends are considered healthy.
type HealthChecker interface {
	IsHealthy(name string) bool
}

// ErrDegraded indicates a backend is marked unhealthy by the health watchdog.
var ErrDegraded = errors.New("backend degraded by health watchdog")

// CheckBackendHealth combines the readiness probe with cached watchdog state.
// When hc reports a backend as unhealthy, live Health() probes are skipped so
// routing and readiness share the same degraded view.
func CheckBackendHealth(ctx context.Context, hc HealthChecker, name string, healthFn func(context.Context) error) error {
	if hc != nil && !hc.IsHealthy(name) {
		return fmt.Errorf("%w: %s", ErrDegraded, name)
	}
	if healthFn == nil {
		return nil
	}
	return healthFn(ctx)
}
