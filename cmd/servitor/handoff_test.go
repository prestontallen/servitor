package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
)

func TestHandoffLine(t *testing.T) {
	h := func(s string) *string { return &s }
	self := api.Author{Actor: "agent:claude", Host: "adirondack", Session: "me"}
	for _, tc := range []struct {
		name    string
		authors []ctxAuthor
		pushed  any
		want    []string // substrings, all required
		not     []string
	}{
		{"no authors", nil, nil, nil, nil},
		{"same session",
			[]ctxAuthor{{Actor: "agent:claude", Host: h("adirondack"), Session: "me", LastTS: "T"}}, nil,
			[]string{"same session: continuing your own work", "last agent:claude on adirondack, session me, T"}, []string{"other agent"}},
		{"new session same host",
			[]ctxAuthor{{Actor: "agent:claude", Host: h("adirondack"), Session: "old"}}, false,
			[]string{"new session on this host", "on this disk"}, []string{"stranded", "other agent"}},
		{"other host, unpushed",
			[]ctxAuthor{{Actor: "agent:claude", Host: h("primarius"), Session: "old"}}, false,
			[]string{"other host primarius", "pushed=false, so the work is stranded on primarius"}, nil},
		{"other host, pushed (string from field.set)",
			[]ctxAuthor{{Actor: "agent:claude", Host: h("primarius"), Session: "old"}}, "true",
			[]string{"other host primarius"}, []string{"stranded"}},
		{"other agent",
			[]ctxAuthor{{Actor: "agent:hermes", Host: h("adirondack"), Session: "h1"}}, nil,
			[]string{"other agent (agent:hermes), new session on this host"}, nil},
		{"human wrote last",
			[]ctxAuthor{{Actor: "human:preston", Host: h("adirondack"), Session: "me"}, {Actor: "agent:claude", Session: "me"}}, nil,
			[]string{"human wrote last: read their events first"}, []string{"same session", "other agent"}},
		{"legacy author",
			[]ctxAuthor{{Actor: "agent:cli"}}, false,
			[]string{"predates host records", "last agent:cli on unknown host, no session"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := handoffLine(self, tc.authors, tc.pushed)
			if tc.want == nil {
				if got != "" {
					t.Errorf("want no line, got %q", got)
				}
				return
			}
			if !strings.HasPrefix(got, "handoff author: ") {
				t.Errorf("prefix: %q", got)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %q", w, got)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in %q", n, got)
				}
			}
		})
	}
}

// TestHandoffLineEmptySessions: with no session on either side, a different
// actor is still named.
func TestHandoffLineEmptySessions(t *testing.T) {
	got := handoffLine(api.Author{Actor: "agent:cli", Host: "adirondack"}, []ctxAuthor{{Actor: "agent:claude"}}, nil)
	if !strings.Contains(got, "other agent (agent:claude)") {
		t.Errorf("empty sessions hid the other agent: %q", got)
	}
}

// TestPreflightHandoffAuthor: the hook header carries the handoff line
// after the handoff fields, and a ticket with no authors prints none.
func TestPreflightHandoffAuthor(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	self := api.Author{Actor: "agent:claude", Host: "adirondack", Session: "new"}
	var b bytes.Buffer
	preflight(&b, dir, self, "", []byte(`{"slug":"x","status":"active","fields":{"pushed":false,"next":"n"},
		"authors":[{"actor":"agent:claude","host":"primarius","session":"old","last_ts":"2026-09-18T00:00:00Z"}]}`))
	out := b.String()
	line := "handoff author: other host primarius: the worktree and any unpushed commits live there; pushed=false, so the work is stranded on primarius until it is pushed (last agent:claude on primarius, session old, 2026-09-18T00:00:00Z)"
	if !strings.Contains(out, line+"\n") {
		t.Errorf("missing line %q in:\n%s", line, out)
	}
	if strings.Index(out, "handoff next") > strings.Index(out, "handoff author") {
		t.Errorf("author line must follow the handoff fields:\n%s", out)
	}
	b.Reset()
	preflight(&b, dir, self, "", []byte(`{"slug":"y","status":"queued","authors":[]}`))
	if strings.Contains(b.String(), "handoff author") {
		t.Errorf("no authors, no line:\n%s", b.String())
	}
}
