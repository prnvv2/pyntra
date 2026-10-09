package scope

import "testing"

func TestMatcherEmptyAllowsAll(t *testing.T) {
	m := NewMatcher(nil, nil, nil, nil)
	if !m.IsEmpty() {
		t.Fatal("empty scope should report IsEmpty")
	}
	if ok, _ := m.Allow("https://anything.example.com"); !ok {
		t.Fatal("empty scope must allow any target")
	}
}

func TestMatcherDomain(t *testing.T) {
	m := NewMatcher([]string{"example.com"}, nil, nil, nil)
	cases := []struct {
		target string
		want   bool
	}{
		{"https://example.com/path", true},
		{"http://sub.example.com", true},
		{"example.com:8443", true},
		{"https://evil.com", false},
		{"https://example.com.evil.com", false}, // suffix trick must be rejected
	}
	for _, c := range cases {
		if ok, _ := m.Allow(c.target); ok != c.want {
			t.Errorf("Allow(%q) = %v, want %v", c.target, ok, c.want)
		}
	}
}

func TestMatcherCIDR(t *testing.T) {
	m := NewMatcher(nil, []string{"10.0.0.0/24", "192.168.1.5"}, nil, nil)
	cases := map[string]bool{
		"10.0.0.17":   true,
		"10.0.1.17":   false,
		"192.168.1.5": true,
		"192.168.1.6": false,
	}
	for target, want := range cases {
		if ok, _ := m.Allow(target); ok != want {
			t.Errorf("Allow(%q) = %v, want %v", target, ok, want)
		}
	}
}

func TestMatcherExclusionWins(t *testing.T) {
	m := NewMatcher([]string{"example.com"}, nil, nil, []string{"secret.example.com"})
	if ok, _ := m.Allow("https://secret.example.com"); ok {
		t.Fatal("exclusion must override an allowing domain rule")
	}
	if ok, _ := m.Allow("https://api.example.com"); !ok {
		t.Fatal("non-excluded in-scope host should be allowed")
	}
}

func TestMatcherNonNetworkTargetAllowed(t *testing.T) {
	m := NewMatcher([]string{"example.com"}, nil, nil, nil)
	// A local file path / non-host argument cannot be evaluated → allow (conservative).
	if ok, _ := m.Allow("/tmp/wordlist.txt"); !ok {
		t.Fatal("non-network argument should not be blocked")
	}
}

func TestExtractTargets(t *testing.T) {
	got := ExtractTargets(map[string]interface{}{
		"url":     "https://a.example.com",
		"targets": "b.example.com, c.example.com",
		"depth":   3,
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 targets, got %v", got)
	}
}
