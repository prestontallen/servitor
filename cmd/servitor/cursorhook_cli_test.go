package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// hook --cursor against a real store: the mode sets the actor when
// SERVITOR_ACTOR is unset, the stdin payload's session_id is the hook's
// session, and the handoff author line reads both back. Proven through the
// verdict: a prior write by agent:cursor in session s1 must read as "same
// session" for a hook fed session_id s1.
func TestHookCursorActorAndSession(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:cursor", Session: "s1",
		Payload: map[string]any{"slug": "cursor-hook", "title": "Cursor"},
	}); err != nil {
		t.Fatal(err)
	}

	old := hookStdin
	t.Cleanup(func() { hookStdin = old })

	hookStdin = strings.NewReader(`{"session_id":"s1","conversation_id":"s1","is_background_agent":false}`)
	out, code := cli(t, svc, nil, "hook", "--cursor", "cursor-hook")
	if code != 0 {
		t.Fatalf("hook --cursor exited %d: %s", code, out)
	}
	var doc map[string]string
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out)
	}
	text := doc["additional_context"]
	if !strings.Contains(text, "same session: continuing your own work") {
		t.Errorf("session s1 from the payload must match the last author:\n%s", text)
	}
	if strings.Contains(text, "other agent") {
		t.Errorf("actor must be agent:cursor without SERVITOR_ACTOR:\n%s", text)
	}
	if !strings.Contains(text, "cursor-hook") {
		t.Errorf("the ticket aggregate must ride inside additional_context:\n%s", text)
	}

	// a different conversation: not the same session
	hookStdin = strings.NewReader(`{"session_id":"s2"}`)
	out, _ = cli(t, svc, nil, "hook", "--cursor", "cursor-hook")
	if !strings.Contains(out, "handoff author:") || strings.Contains(out, "same session") {
		t.Errorf("session s2 must not read as the same session:\n%s", out)
	}

	// SERVITOR_ACTOR still wins over the mode
	hookStdin = strings.NewReader(`{"session_id":"s1"}`)
	out, _ = cli(t, svc, map[string]string{"SERVITOR_ACTOR": "agent:other"}, "hook", "--cursor", "cursor-hook")
	if !strings.Contains(out, "other agent (agent:cursor)") {
		t.Errorf("SERVITOR_ACTOR=agent:other must be the hook's actor:\n%s", out)
	}
}
