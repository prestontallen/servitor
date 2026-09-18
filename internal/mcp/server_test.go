package mcp

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

func testService(t *testing.T) api.Service {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, "postgres://postgres:psql@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	defer admin.Close(ctx)
	for _, q := range []string{
		`DROP DATABASE IF EXISTS servitor_mcp_test WITH (FORCE)`,
		`CREATE DATABASE servitor_mcp_test`,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	s, err := store.Open(ctx, "postgres://postgres:psql@localhost:5432/servitor_mcp_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Pool.Close() })
	return api.NewStoreService(s)
}

// roundtrip feeds framed JSON-RPC through the server and collects responses.
func roundtrip(t *testing.T, svc api.Service, frames ...string) []string {
	t.Helper()
	in := strings.Join(frames, "\n")
	var out strings.Builder
	srv := &Server{Service: svc, In: strings.NewReader(in), Out: &out, Err: io.Discard}
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	var resp []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line != "" {
			resp = append(resp, line)
		}
	}
	return resp
}

func TestMCPInitializeAndToolsList(t *testing.T) {
	svc := testService(t)
	resp := roundtrip(t, svc,
		`{"id":1,"method":"initialize","params":{}}`,
		`{"id":2,"method":"tools/list"}`)
	if len(resp) != 2 {
		t.Fatalf("got %d responses", len(resp))
	}
	if !strings.Contains(resp[0], `"protocolVersion"`) {
		t.Errorf("initialize response: %s", resp[0])
	}
	if !strings.Contains(resp[1], "servitor_append") || !strings.Contains(resp[1], "servitor_ctx") {
		t.Errorf("tools/list missing tools: %s", resp[1])
	}
}

func TestMCPCtxTool(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: store.NewULID(), Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "mcp-test", "title": "MCP"},
	}); err != nil {
		t.Fatal(err)
	}
	resp := roundtrip(t, svc,
		`{"id":1,"method":"tools/call","params":{"name":"servitor_ctx","arguments":{"ref":"mcp-test"}}}`)
	if len(resp) != 1 || !strings.Contains(resp[0], "mcp-test") {
		t.Errorf("ctx tool failed: %v", resp)
	}
}

func TestMCPAppendToolAndStableErrors(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "mcp-write", "title": "W"},
	}); err != nil {
		t.Fatal(err)
	}

	// create a second ticket via the append tool (payload.ulid form)
	newID := store.NewULID()
	resp := roundtrip(t, svc,
		`{"id":1,"method":"tools/call","params":{"name":"servitor_append","arguments":{"kind":"ticket.create","payload":{"ulid":"`+newID+`","slug":"mcp-new"},"actor":"agent:claude"}}}`)
	if len(resp) != 1 || strings.Contains(resp[0], `"error"`) {
		t.Errorf("ticket.create via MCP failed: %v", resp)
	}

	// domain violation surfaces as a tool error with the stable code
	resp = roundtrip(t, svc,
		`{"id":2,"method":"tools/call","params":{"name":"servitor_append","arguments":{"kind":"gate","ticket":"mcp-write","payload":{"gate":"contract_approved"},"actor":"agent:claude"}}}`)
	if len(resp) != 1 || !strings.Contains(resp[0], "human_gate_required") {
		t.Errorf("stable error code missing: %v", resp)
	}

	// note append then ctx shows it
	resp = roundtrip(t, svc,
		`{"id":3,"method":"tools/call","params":{"name":"servitor_append","arguments":{"kind":"note","ticket":"mcp-write","payload":{"v":"from mcp"},"actor":"agent:claude"}}}`)
	if len(resp) != 1 || strings.Contains(resp[0], `"error"`) {
		t.Errorf("note append failed: %v", resp)
	}
	resp = roundtrip(t, svc,
		`{"id":4,"method":"tools/call","params":{"name":"servitor_ctx","arguments":{"ref":"mcp-write"}}}`)
	if !strings.Contains(resp[0], "from mcp") {
		t.Errorf("note not in ctx: %s", resp[0])
	}
}
