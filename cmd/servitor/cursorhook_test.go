package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCursorSession(t *testing.T) {
	cases := []struct{ name, stdin, want string }{
		{"session_id", `{"session_id":"s1","is_background_agent":false}`, "s1"},
		{"conversation_id only", `{"conversation_id":"c1"}`, "c1"},
		{"session_id wins", `{"session_id":"s1","conversation_id":"c1"}`, "s1"},
		{"neither", `{"composer_mode":"agent"}`, ""},
		{"malformed json", `{not json`, ""},
		{"empty stdin", ``, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cursorSession(strings.NewReader(tc.stdin)); got != tc.want {
				t.Errorf("cursorSession(%q) = %q, want %q", tc.stdin, got, tc.want)
			}
		})
	}
}

func TestEmitCursorContext(t *testing.T) {
	// the hook output comes back as one JSON object with additional_context
	var out bytes.Buffer
	text := "cwd: /x\nworktree /w on main\n"
	if code := emitCursorContext(&out, []byte(text)); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var doc map[string]string
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out.String())
	}
	if doc["additional_context"] != strings.TrimSpace(text) {
		t.Errorf("additional_context = %q, want the hook text", doc["additional_context"])
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Error("output must end with a newline")
	}

	// empty hook output: silent, still exit 0
	out.Reset()
	if code := emitCursorContext(&out, nil); code != 0 || out.Len() != 0 {
		t.Errorf("empty hook output must print nothing (code=%d, out=%q)", code, out.String())
	}
}
