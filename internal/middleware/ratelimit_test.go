package middleware

import "testing"

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(10, 5) // 10 rps, burst of 5

	// First 5 requests should be allowed (burst).
	for i := 0; i < 5; i++ {
		remaining, ok := rl.Allow("key-1")
		if !ok {
			t.Fatalf("request %d: expected allow, got deny", i+1)
		}
		if remaining != 5-i-1 {
			t.Errorf("request %d: remaining = %d, want %d", i+1, remaining, 5-i-1)
		}
	}

	// 6th request should be denied (burst exhausted).
	_, ok := rl.Allow("key-1")
	if ok {
		t.Error("request 6: expected deny after burst exhausted, got allow")
	}
}

func TestRateLimiterPerKey(t *testing.T) {
	rl := NewRateLimiter(10, 2) // burst of 2

	// Exhaust key-1.
	rl.Allow("key-1")
	rl.Allow("key-1")
	_, ok := rl.Allow("key-1")
	if ok {
		t.Error("key-1 should be denied after burst")
	}

	// key-2 should still have its own bucket.
	_, ok = rl.Allow("key-2")
	if !ok {
		t.Error("key-2 should be allowed (independent bucket)")
	}
}

func TestRateLimiterNewKeyGetsBurst(t *testing.T) {
	rl := NewRateLimiter(1, 3)

	remaining, ok := rl.Allow("fresh-key")
	if !ok {
		t.Fatal("new key should be allowed")
	}
	if remaining != 2 {
		t.Errorf("remaining = %d, want 2", remaining)
	}
}
