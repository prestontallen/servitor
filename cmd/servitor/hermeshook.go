package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// hermesPayload matches the stdin wire of Hermes shell hooks
// (~/.hermes/hermes-agent/agent/shell_hooks.py, _payload_fields): only
// tool_name/args/session_id/parent_session_id are promoted to top-level
// keys; every other callback kwarg rides under "extra" — including
// is_first_turn and user_message for pre_llm_call.
type hermesPayload struct {
	HookEventName string `json:"hook_event_name"`
	Extra         struct {
		IsFirstTurn json.RawMessage `json:"is_first_turn"`
	} `json:"extra"`
}

// agentName picks the wording for host-specific hints. SERVITOR_AGENT wins
// (the explicit override the contract fixed), else the actor's agent name;
// Claude stays the default for anything unrecognised.
func agentName(actor, agentEnv string) string {
	if agentEnv != "" {
		return agentEnv
	}
	if n := strings.TrimPrefix(actor, "agent:"); n != actor && n != "" {
		return n
	}
	return "claude"
}

// firstTurn reports whether the hook payload says this is the session's
// first LLM call. The hook must never block or starve a session, so every
// ambiguous case (empty stdin, unparsable JSON, absent flag, unparsable
// value) degrades to first turn — the standalone `servitor hook` case, which
// always speaks. Only an explicit falsy value suppresses.
func firstTurn(stdin io.Reader) bool {
	b, err := io.ReadAll(stdin)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return true
	}
	var p hermesPayload
	if json.Unmarshal(b, &p) != nil {
		return true
	}
	raw := p.Extra.IsFirstTurn
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var bl bool
	if json.Unmarshal(raw, &bl) == nil {
		return bl
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return true
}

// emitHermesContext wraps the hook output in the Hermes pre_llm_call
// context-injection shape ({"context": ...}) on the session's first turn and
// prints nothing otherwise. Both paths exit 0: an injection problem is the
// session's problem to notice, never its blocker.
func emitHermesContext(out io.Writer, hookOut []byte, stdin io.Reader) int {
	if !firstTurn(stdin) {
		return 0
	}
	text := strings.TrimSpace(string(hookOut))
	if text == "" {
		return 0
	}
	doc, err := json.Marshal(map[string]string{"context": text})
	if err != nil {
		return 0
	}
	out.Write(doc)
	fmt.Fprintln(out)
	return 0
}
