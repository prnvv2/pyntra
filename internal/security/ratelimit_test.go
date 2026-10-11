package security

import (
	"context"
	"testing"
	"time"
)

func TestHostKey(t *testing.T) {
	cases := map[string]string{
		"https://a.example.com/p": "a.example.com",
		"http://10.0.0.5:8080/x":  "10.0.0.5",
		"b.example.com:443":       "b.example.com",
		"c.example.com":           "c.example.com",
		"":                        "",
	}
	for in, want := range cases {
		if got := HostKey(in); got != want {
			t.Errorf("HostKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostRateLimiter_Throttles(t *testing.T) {
	l := NewHostRateLimiter(10) // 10 rps, burst ~10
	ctx := context.Background()
	start := time.Now()
	// Exhaust the burst, then one more must wait ~1/10s.
	for i := 0; i < 12; i++ {
		if err := l.Wait(ctx, "h.example.com"); err != nil {
			t.Fatalf("Wait error: %v", err)
		}
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("expected limiter to introduce delay, elapsed %v", time.Since(start))
	}
}

func TestHostRateLimiter_DisabledAndEmptyHost(t *testing.T) {
	// rps <= 0 disables limiting.
	l := NewHostRateLimiter(0)
	if err := l.Wait(context.Background(), "h"); err != nil {
		t.Errorf("disabled limiter should not error: %v", err)
	}
	// Empty host is a no-op even when enabled.
	l2 := NewHostRateLimiter(1)
	if err := l2.Wait(context.Background(), ""); err != nil {
		t.Errorf("empty host should be a no-op: %v", err)
	}
}
