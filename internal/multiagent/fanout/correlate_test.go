package fanout

import (
	"strings"
	"testing"

	"pyntra/internal/database"
)

func TestCorrelate_CredentialReuseAndSystemicCWE(t *testing.T) {
	findings := []*database.Vulnerability{
		{Title: "Hardcoded API key in JS", Target: "https://a.example.com", Proof: "found api key AKIA...", CWEIDs: []string{"CWE-798"}},
		{Title: "SQL injection", Target: "https://b.example.com/login", CWEIDs: []string{"CWE-89"}},
		{Title: "SQL injection in search", Target: "https://a.example.com/q", CWEIDs: []string{"CWE-89"}},
	}
	out := correlate(findings)
	if out == "" {
		t.Fatal("expected a correlation section")
	}
	if !strings.Contains(out, "Credential") {
		t.Errorf("expected credential-reuse correlation, got:\n%s", out)
	}
	if !strings.Contains(out, "CWE-89") {
		t.Errorf("expected systemic CWE-89 correlation, got:\n%s", out)
	}
	// a.example.com and b.example.com both carry CWE-89.
	if !strings.Contains(out, "b.example.com") {
		t.Errorf("expected spray candidate b.example.com, got:\n%s", out)
	}
}

func TestCorrelate_NothingToCorrelate(t *testing.T) {
	findings := []*database.Vulnerability{
		{Title: "Clickjacking", Target: "https://a.example.com", CWEIDs: []string{"CWE-1021"}},
	}
	if out := correlate(findings); out != "" {
		t.Errorf("expected empty correlation, got:\n%s", out)
	}
}

func TestTargetHost(t *testing.T) {
	cases := map[string]string{
		"https://a.example.com/path?x=1": "a.example.com",
		"http://10.0.0.5:8080":           "10.0.0.5",
		"b.example.com":                  "b.example.com",
		"":                               "",
	}
	for in, want := range cases {
		if got := targetHost(in); got != want {
			t.Errorf("targetHost(%q) = %q, want %q", in, got, want)
		}
	}
}
