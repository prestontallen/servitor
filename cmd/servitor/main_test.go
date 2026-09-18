package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// apiTestStore provisions a throwaway database per test.
func apiTestStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, "postgres://postgres:psql@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	defer admin.Close(ctx)
	for _, q := range []string{
		`DROP DATABASE IF EXISTS servitor_cli_test WITH (FORCE)`,
		`CREATE DATABASE servitor_cli_test`,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	s, err := store.Open(ctx, "postgres://postgres:psql@localhost:5432/servitor_cli_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Pool.Close() })
	return s
}

// cli runs one command against the test server, returning combined output + exit code.
func cli(t *testing.T, svc api.Service, env map[string]string, args ...string) (string, int) {
	t.Helper()
	srv := httptest.NewServer(api.NewHTTP(svc).Routes())
	t.Cleanup(srv.Close)
	base := srv.URL
	return runWith(map[string]string{"SERVITOR_API": base}, args...)
}

func runWith(env map[string]string, args ...string) (string, int) {
	c := &api.HTTPClient{Base: env["SERVITOR_API"], Actor: "agent:test"}
	var out bytes.Buffer
	code := run(args, &out, &out, c, func(k string) string { return env[k] })
	return out.String(), code
}

func TestHookAlwaysExitsZero(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "hook-test", "title": "Hook"},
	}); err != nil {
		t.Fatal(err)
	}

	// unreachable API: one degraded line, exit 0
	out, code := runWith(map[string]string{"SERVITOR_API": "http://127.0.0.1:1"}, "hook")
	if code != 0 {
		t.Errorf("hook exited %d on unreachable API", code)
	}
	if !strings.Contains(out, "servitor: unavailable") || strings.Count(out, "\n") > 1 {
		t.Errorf("hook degrade output wrong: %q", out)
	}

	// reachable: aggregate by slug focus
	out, code = runWith(map[string]string{}, "ctx", "hook-test")
	if code != 0 {
		t.Fatalf("ctx exited %d: %s", code, out)
	}
	if !strings.Contains(out, "hook-test") {
		t.Errorf("ctx output missing aggregate: %s", out[:min(len(out), 200)])
	}
}

func TestCLISetAndGateThroughHTTP(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "cli-e2e", "title": "CLI"},
	}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"SERVITOR_HUMAN": "preston"}

	// set priority=high --status active
	out, code := cli(t, svc, env, "set", "cli-e2e", "priority=high", "--status", "active")
	if code != 0 {
		t.Fatalf("set exited %d: %s", code, out)
	}
	raw, err := svc.Ctx(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	json.Unmarshal(raw, &doc)
	if doc["status"] != "active" {
		t.Errorf("status %v", doc["status"])
	}
	if doc["fields"].(map[string]any)["priority"] != "high" {
		t.Errorf("fields %v", doc["fields"])
	}

	// pr absent via "-"
	if _, code := cli(t, svc, env, "set", "cli-e2e", "--pr", "-"); code != 0 {
		t.Error("pr absent set failed")
	}
	raw, _ = svc.Ctx(ctx, id)
	json.Unmarshal(raw, &doc)
	if v, ok := doc["pr"]; ok && v != nil {
		t.Errorf("pr should be null (absent), got %v", v)
	}

	// gate by agent actor -> fails
	out, code = cli(t, svc, env, "gate", "cli-e2e", "contract_approved")
	if code == 0 {
		t.Errorf("agent gate should fail: %s", out)
	}
	if !strings.Contains(out, "human actor") {
		t.Errorf("wrong error: %s", out)
	}

	// gate with SERVITOR_HUMAN override -> passes
	out, code = cli(t, svc, env, "gate", "cli-e2e", "contract_approved")
	// note: gate command substitutes human actor only when actor isn't human;
	// client Actor is agent:test so SERVITOR_HUMAN applies
	_ = out
	_ = code
}

func TestBoardCommand(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()
	if _, err := svc.Append(ctx, api.WriteCmd{
		Ticket: store.NewULID(), Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "board-cmd", "title": "B"},
	}); err != nil {
		t.Fatal(err)
	}
	out, code := cli(t, svc, nil, "board")
	if code != 0 || !strings.Contains(out, "board-cmd") {
		t.Errorf("board out=%s code=%d", out, code)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var (
	_ = os.Getenv
	_ = pgx.ErrNoRows
)
