package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
	"github.com/prestontallen/servitor/internal/testdb"
)

func testService(t *testing.T) api.Service {
	t.Helper()
	ctx := context.Background()
	adminDSN := testdb.AdminDSN(t)
	admin, err := pgx.Connect(ctx, adminDSN)
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
	s, err := store.Open(ctx, testdb.Named(t, adminDSN, "servitor_mcp_test"))
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
		`{"id":1,"method":"tools/call","params":{"name":"servitor_append","arguments":{"kind":"ticket.create","payload":{"ulid":"`+newID+`","slug":"mcp-new","tier":1},"actor":"agent:claude"}}}`)
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

// TestMCPCreateRequiresTier: the append tool refuses ticket.create without a
// tier the store would accept, with the stable code and before any write;
// with one, the ticket reads back classified.
func TestMCPCreateRequiresTier(t *testing.T) {
	svc := testService(t)
	for i, payload := range []string{
		`{"slug":"mcp-untiered"}`,
		`{"slug":"mcp-untiered","tier":4}`,
		`{"slug":"mcp-untiered","tier":"two"}`,
		`{"slug":"mcp-untiered","tier":1.5}`,
	} {
		resp := roundtrip(t, svc,
			`{"id":`+string(rune('1'+i))+`,"method":"tools/call","params":{"name":"servitor_append","arguments":{"kind":"ticket.create","payload":`+payload+`,"actor":"agent:claude"}}}`)
		if len(resp) != 1 || !strings.Contains(resp[0], "tier_required") {
			t.Fatalf("payload %s: want tier_required, got %v", payload, resp)
		}
	}
	if _, err := svc.Ctx(context.Background(), "mcp-untiered"); err == nil {
		t.Fatal("a ticket exists after refused creates")
	}
	resp := roundtrip(t, svc,
		`{"id":9,"method":"tools/call","params":{"name":"servitor_append","arguments":{"kind":"ticket.create","payload":{"slug":"mcp-tiered","tier":"2"},"actor":"agent:claude"}}}`)
	if len(resp) != 1 || strings.Contains(resp[0], `"error"`) {
		t.Fatalf("create with tier failed: %v", resp)
	}
	raw, err := svc.Ctx(context.Background(), "mcp-tiered")
	if err != nil {
		t.Fatal(err)
	}
	var agg struct {
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(raw, &agg); err != nil {
		t.Fatal(err)
	}
	if got := agg.Fields["tier"]; got != "2" {
		t.Fatalf("fields.tier after create with tier \"2\" = %#v, want \"2\"", got)
	}
}
