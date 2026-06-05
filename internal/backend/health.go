package backend

// HealthChecker reports whether a backend should receive new inference traffic.
// A nil HealthChecker means all backends are considered healthy.
type HealthChecker interface {
	IsHealthy(name string) bool
}
