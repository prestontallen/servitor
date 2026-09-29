package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// TestCLIStampsAuthor: the client built the way main builds it carries the
// harness identity to the ledger row: actor, host and session.
func TestCLIStampsAuthor(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "author-e2e", "title": "author"},
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.NewHTTP(svc).Routes())
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name                 string
		env                  map[string]string
		actor, host, session string
	}{
		{"claude", map[string]string{"CLAUDE_CODE_SESSION_ID": "S", "SERVITOR_HOST": "Adirondack"}, "agent:claude", "adirondack", "S"},
		{"hermes", map[string]string{"HERMES_SESSION_ID": "H", "SERVITOR_HOST": "adirondack"}, "agent:hermes", "adirondack", "H"},
		{"manual", map[string]string{"SERVITOR_HOST": "x"}, "agent:cli", "x", ""},
		{"explicit actor", map[string]string{"SERVITOR_ACTOR": "human:preston", "CLAUDE_CODE_SESSION_ID": "S", "SERVITOR_HOST": "x"}, "human:preston", "x", "S"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(k string) string { return tc.env[k] }
			a := api.LocalAuthor(getenv)
			c := &api.HTTPClient{Base: srv.URL, Actor: a.Actor, Host: a.Host, Session: a.Session}
			var out bytes.Buffer
			if code := run([]string{"log", "author-e2e", "note", tc.name}, &out, &out, c, getenv); code != 0 {
				t.Fatalf("log exited %d: %s", code, out.String())
			}
			evs, err := svc.History(ctx, id, 0)
			if err != nil {
				t.Fatal(err)
			}
			e := evs[len(evs)-1]
			if e.Actor != tc.actor || e.Host == nil || *e.Host != tc.host || e.Session == nil || *e.Session != tc.session {
				t.Errorf("row actor %q host %v session %v; want %q %q %q", e.Actor, e.Host, e.Session, tc.actor, tc.host, tc.session)
			}
		})
	}
}
