package api

import (
	"os"
	"testing"
)

func TestLocalAuthor(t *testing.T) {
	osHost, _ := os.Hostname()
	for _, tc := range []struct {
		name string
		env  map[string]string
		want Author
	}{
		{"claude", map[string]string{"CLAUDE_CODE_SESSION_ID": "c1"},
			Author{"agent:claude", NormHost(osHost), "c1"}},
		{"hermes", map[string]string{"HERMES_SESSION_ID": "h1"},
			Author{"agent:hermes", NormHost(osHost), "h1"}},
		{"hermes inside a claude shell", map[string]string{"HERMES_SESSION_ID": "h1", "CLAUDE_CODE_SESSION_ID": "c1"},
			Author{"agent:hermes", NormHost(osHost), "h1"}},
		{"no harness", map[string]string{},
			Author{"agent:cli", NormHost(osHost), ""}},
		{"explicit actor wins, session still stamped", map[string]string{"SERVITOR_ACTOR": "human:preston", "CLAUDE_CODE_SESSION_ID": "c1"},
			Author{"human:preston", NormHost(osHost), "c1"}},
		{"host override", map[string]string{"SERVITOR_HOST": "Adirondack.local", "CLAUDE_CODE_SESSION_ID": "c1"},
			Author{"agent:claude", "adirondack", "c1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := LocalAuthor(func(k string) string { return tc.env[k] })
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
	if osHost != "" && LocalAuthor(func(string) string { return "" }).Host == "" {
		t.Error("host not stamped from os.Hostname")
	}
}

func TestNormHost(t *testing.T) {
	for in, want := range map[string]string{
		"Adirondack":        "adirondack",
		"primarius.local":   "primarius",
		" MacBook-Pro.lan ": "macbook-pro",
		"":                  "",
		".weird":            ".weird",
	} {
		if got := NormHost(in); got != want {
			t.Errorf("NormHost(%q) = %q, want %q", in, got, want)
		}
	}
}
