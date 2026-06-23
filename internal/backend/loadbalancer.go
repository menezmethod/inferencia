package backend

import "sync"

// LoadBalancer tracks in-flight requests per backend and selects the least-loaded
// candidate. When multiple backends have the same load, selection round-robins.
type LoadBalancer struct {
	mu     sync.Mutex
	active map[string]int
	rr     uint64
}

// NewLoadBalancer creates a LoadBalancer.
func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{active: make(map[string]int)}
}

// Acquire increments the in-flight count for name.
func (lb *LoadBalancer) Acquire(name string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.active[name]++
}

// Release decrements the in-flight count for name.
func (lb *LoadBalancer) Release(name string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if lb.active[name] > 0 {
		lb.active[name]--
	}
}

// Select picks the backend with the fewest in-flight requests from names.
// Ties are broken with round-robin. The caller must call Acquire for the
// returned name before dispatching the request.
func (lb *LoadBalancer) Select(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	minLoad := lb.active[names[0]]
	for _, name := range names[1:] {
		if load := lb.active[name]; load < minLoad {
			minLoad = load
		}
	}

	var tied []string
	for _, name := range names {
		if lb.active[name] == minLoad {
			tied = append(tied, name)
		}
	}

	pick := tied[lb.rr%uint64(len(tied))]
	lb.rr++
	return pick
}

// InFlight returns the current in-flight count for name (for tests).
func (lb *LoadBalancer) InFlight(name string) int {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.active[name]
}
