package config

import (
	"os"
	"testing"
)

func TestIsInsecureRemoteBaseURL(t *testing.T) {
	cases := map[string]bool{
		"":                            false,
		"https://api.openai.com/v1":   false, // https remote: fine
		"http://localhost:11434/v1":   false, // local http: fine (Ollama)
		"http://127.0.0.1:1234/v1":    false,
		"http://192.168.1.10:8000/v1": false, // private network: fine
		"http://api.example.com/v1":   true,  // remote plain http: INSECURE
		"http://1.2.3.4/v1":           true,  // public IP http: INSECURE
	}
	for url, want := range cases {
		if got := IsInsecureRemoteBaseURL(url); got != want {
			t.Errorf("IsInsecureRemoteBaseURL(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	cases := map[string]bool{
		"localhost":        true,
		"app.localhost":    true,
		"foo.test":         true,
		"127.0.0.1":        true,
		"10.1.2.3":         true,
		"192.168.0.1:8080": true,
		"8.8.8.8":          false,
		"example.com":      false,
	}
	for host, want := range cases {
		if got := IsLocalHost(host); got != want {
			t.Errorf("IsLocalHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestExpandEnvRef(t *testing.T) {
	os.Setenv("PYNTRA_TEST_KEY", "sk-secret")
	defer os.Unsetenv("PYNTRA_TEST_KEY")
	if got := expandEnvRef("${PYNTRA_TEST_KEY}"); got != "sk-secret" {
		t.Errorf("expandEnvRef expansion = %q", got)
	}
	// A literal value not starting with $ is returned unchanged (passwords with $ mid-string are safe).
	if got := expandEnvRef("A-Literal-Secret"); got != "A-Literal-Secret" {
		t.Errorf("literal value altered: %q", got)
	}
}
