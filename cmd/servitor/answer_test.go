package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// TestAnswerVerb: servitor answer closes a question by identity. One
// subitem.set carrying the answer as state, addressed by the prefix add
// printed; the actor is whoever ran it, so a human answer rides on
// SERVITOR_ACTOR the way the contract gate does. Short calls exit 2 before
// any write.
func TestAnswerVerb(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "cli-answer", "title": "CLI answer", "tier": 1},
	}); err != nil {
		t.Fatal(err)
	}
	out, code := cli(t, svc, nil, "add", "cli-answer", "question", "which modules?")
	if code != 0 {
		t.Fatalf("add exited %d: %s", code, out)
	}
	q := strings.TrimSpace(out)
	before, err := svc.History(ctx, "cli-answer", 0)
	if err != nil {
		t.Fatal(err)
	}

	// too few args: usage, exit 2, nothing written
	for _, args := range [][]string{{"answer"}, {"answer", "cli-answer"}, {"answer", "cli-answer", q[:12]}} {
		out, code := cli(t, svc, nil, args...)
		if code != 2 || !strings.Contains(out, "<text>") {
			t.Fatalf("%v: exit %d, output %q; want exit 2 naming the usage", args, code, out)
		}
	}
	after, err := svc.History(ctx, "cli-answer", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("a refused answer wrote %d event(s)", len(after)-len(before))
	}

	// the human answers: multi-word text joins, actor is the human. The
	// client's Actor is what SERVITOR_ACTOR resolves to in a real run; the
	// test harness pins it, so runAs sets it the way the policy tests do.
	srv := httptest.NewServer(api.NewHTTP(svc).Routes())
	t.Cleanup(srv.Close)
	out, code = runAs("human:preston", map[string]string{"SERVITOR_API": srv.URL},
		"answer", "cli-answer", q[:12], "AutoStore", "and", "JustSleep")
	if code != 0 {
		t.Fatalf("answer exited %d: %s", code, out)
	}
	var state string
	if err := st.Pool.QueryRow(ctx, `SELECT state FROM subitems WHERE ulid=$1`, q).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "AutoStore and JustSleep" {
		t.Errorf("question state %q, want the joined answer", state)
	}
	evs, err := svc.History(ctx, "cli-answer", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != len(before)+1 {
		t.Fatalf("answer wrote %d events, want exactly one", len(evs)-len(before))
	}
	last := evs[len(evs)-1]
	if last.Kind != "subitem.set" || last.Actor != "human:preston" {
		t.Fatalf("answer event = %s by %s, want subitem.set by human:preston", last.Kind, last.Actor)
	}
	if last.Payload["state"] != "AutoStore and JustSleep" || last.Payload["ulid"] != q[:12] {
		t.Errorf("answer payload %v, want state=text and the prefix given", last.Payload)
	}

	// and ctx carries it back as the question's state
	raw, err := svc.Ctx(ctx, "cli-answer")
	if err != nil {
		t.Fatal(err)
	}
	var agg struct {
		Questions []struct {
			State *string `json:"state"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(raw, &agg); err != nil {
		t.Fatal(err)
	}
	if len(agg.Questions) != 1 || agg.Questions[0].State == nil || *agg.Questions[0].State != "AutoStore and JustSleep" {
		t.Errorf("ctx questions = %+v, want one with the answer as state", agg.Questions)
	}
}
