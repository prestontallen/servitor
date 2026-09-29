package mcp

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// TestMCPAppendStampsHost: the MCP server stamps its own host on writes and
// fills the session only when the caller sent none; an empty session is not
// an error.
func TestMCPAppendStampsHost(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := svc.Append(ctx, api.WriteCmd{Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "mcp-author"}}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []api.Author{{Host: "adirondack"}, {Host: "adirondack", Session: "env-S"}} {
		var out strings.Builder
		srv := &Server{Service: svc, Out: &out, Err: io.Discard, Author: a,
			In: strings.NewReader(`{"id":1,"method":"tools/call","params":{"name":"servitor_append","arguments":{"kind":"note","ticket":"mcp-author","payload":{"v":"x"},"actor":"agent:claude"}}}`)}
		if err := srv.Serve(ctx); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), `"isError":true`) {
			t.Fatalf("append failed: %s", out.String())
		}
	}
	evs, err := svc.History(ctx, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"", "env-S"} {
		e := evs[len(evs)-2+i]
		if e.Host == nil || *e.Host != "adirondack" || e.Session == nil || *e.Session != want {
			t.Errorf("row %d: host %v session %v, want session %q", i, e.Host, e.Session, want)
		}
	}
}
