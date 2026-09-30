package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// hookStdin is the hook payload source; tests swap it for a reader.
var hookStdin io.Reader = os.Stdin

// cursorPayload matches the stdin wire of a Cursor sessionStart hook
// (cursor.com/docs/agent/hooks): session_id is documented as the same value
// as conversation_id; both are read so either spelling stamps the session.
type cursorPayload struct {
	SessionID      string `json:"session_id"`
	ConversationID string `json:"conversation_id"`
}

// cursorSession reads the sessionStart payload and returns the conversation
// id, or "" when stdin is empty, unparsable, or carries neither field. The
// hook never blocks a session, so every ambiguous case is "no session".
func cursorSession(stdin io.Reader) string {
	b, err := io.ReadAll(stdin)
	if err != nil {
		return ""
	}
	var p cursorPayload
	if json.Unmarshal(b, &p) != nil {
		return ""
	}
	if p.SessionID != "" {
		return p.SessionID
	}
	return p.ConversationID
}

// emitCursorContext wraps the hook output in the Cursor sessionStart
// response shape ({"additional_context": ...}). sessionStart fires once per
// conversation, so unlike Hermes there is no first-turn gate. Exit 0 either
// way: an injection problem is the session's problem to notice, never its
// blocker.
func emitCursorContext(out io.Writer, hookOut []byte) int {
	text := strings.TrimSpace(string(hookOut))
	if text == "" {
		return 0
	}
	doc, err := json.Marshal(map[string]string{"additional_context": text})
	if err != nil {
		return 0
	}
	out.Write(doc)
	fmt.Fprintln(out)
	return 0
}
