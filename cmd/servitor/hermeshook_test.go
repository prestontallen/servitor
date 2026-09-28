package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestFirstTurn(t *testing.T) {
	cases := []struct {
		name, stdin string
		want        bool
	}{
		{"explicit true", `{"hook_event_name":"pre_llm_call","extra":{"is_first_turn":true}}`, true},
		{"explicit false", `{"hook_event_name":"pre_llm_call","extra":{"is_first_turn":false}}`, false},
		{"string true", `{"extra":{"is_first_turn":"true"}}`, true},
		{"string false", `{"extra":{"is_first_turn":"false"}}`, false},
		{"absent flag", `{"hook_event_name":"pre_llm_call","extra":{}}`, true},
		{"no extra", `{"hook_event_name":"pre_llm_call"}`, true},
		{"malformed json", `{not json`, true},
		{"empty stdin", ``, true},
		{"garbage value", `{"extra":{"is_first_turn":null}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstTurn(strings.NewReader(tc.stdin)); got != tc.want {
				t.Errorf("firstTurn(%q) = %v, want %v", tc.stdin, got, tc.want)
			}
		})
	}
}

func TestAgentName(t *testing.T) {
	cases := []struct{ actor, env, want string }{
		{"agent:hermes", "", "hermes"},
		{"agent:cli", "hermes", "hermes"}, // env override wins
		{"agent:claude", "", "claude"},
		{"agent:cli", "", "cli"}, // unknown agent: the wording switch falls back to Claude
		{"", "hermes", "hermes"},
		{"", "", "claude"},
		{"hermes", "", "claude"}, // bare name without agent: prefix
	}
	for _, tc := range cases {
		if got := agentName(tc.actor, tc.env); got != tc.want {
			t.Errorf("agentName(%q, %q) = %q, want %q", tc.actor, tc.env, got, tc.want)
		}
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
	return string(out)
}

func TestPreflightWorktreeHint(t *testing.T) {
	// a canonical checkout on main prints the worktree guidance; the
	// wording must follow the agent
	repo := t.TempDir()
	gitOut(t, repo, "init", "-q")
	gitOut(t, repo, "config", "user.email", "t@t")
	gitOut(t, repo, "config", "user.name", "t")
	gitOut(t, repo, "commit", "-q", "--allow-empty", "-m", "init")

	var b bytes.Buffer
	preflight(&b, repo, "agent:hermes", "", nil)
	if !strings.Contains(b.String(), "Hermes: then cd ../") {
		t.Errorf("actor agent:hermes wants Hermes wording, got:\n%s", b.String())
	}
	if strings.Contains(b.String(), "EnterWorktree") {
		t.Errorf("actor agent:hermes must not get the Claude line:\n%s", b.String())
	}

	b.Reset()
	preflight(&b, repo, "agent:claude", "", nil)
	if !strings.Contains(b.String(), "Claude Code: then EnterWorktree") {
		t.Errorf("actor agent:claude wants the Claude line, got:\n%s", b.String())
	}

	// the env override beats the actor
	b.Reset()
	preflight(&b, repo, "agent:cli", "hermes", nil)
	if !strings.Contains(b.String(), "Hermes: then cd ../") {
		t.Errorf("SERVITOR_AGENT=hermes override wants Hermes wording, got:\n%s", b.String())
	}
}

func TestEmitHermesContext(t *testing.T) {
	// first turn: the hook output comes back wrapped in the injection shape
	var out bytes.Buffer
	code := emitHermesContext(&out, []byte("cwd: /x\nworktree /w on main\n"),
		strings.NewReader(`{"hook_event_name":"pre_llm_call","extra":{"is_first_turn":true}}`))
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.HasPrefix(out.String(), `{"context":`) || !strings.Contains(out.String(), "cwd: /x") {
		t.Errorf("want wrapped context JSON, got: %s", out.String())
	}

	// later turn: silent
	out.Reset()
	code = emitHermesContext(&out, []byte("cwd: /x"),
		strings.NewReader(`{"extra":{"is_first_turn":false}}`))
	if code != 0 || out.Len() != 0 {
		t.Errorf("later turn must print nothing (code=%d, out=%q)", code, out.String())
	}

	// empty hook output: silent even on the first turn
	out.Reset()
	code = emitHermesContext(&out, nil, strings.NewReader(`{"extra":{"is_first_turn":true}}`))
	if code != 0 || out.Len() != 0 {
		t.Errorf("empty hook output must print nothing (code=%d, out=%q)", code, out.String())
	}
}
