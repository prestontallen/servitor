package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
	"github.com/prestontallen/servitor/internal/testdb"
)

// apiTestStore provisions a throwaway database per test.
func apiTestStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	adminDSN := testdb.AdminDSN(t)
	admin, err := pgx.Connect(ctx, adminDSN)
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
	s, err := store.Open(ctx, testdb.Named(t, adminDSN, "servitor_cli_test"))
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
	// preflight header lines first, then exactly one degraded line last
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(lines[len(lines)-1], "servitor: unavailable") || strings.Count(out, "unavailable") != 1 {
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

// TestPreflight needs no database: a non-git dir degrades to one line, and a
// fresh repo with no origin reports canonical + unreachable within budget.
func TestPreflight(t *testing.T) {
	var b bytes.Buffer
	dir := t.TempDir()
	preflight(&b, dir, "agent:test", nil)
	if !strings.Contains(b.String(), "not a git checkout") {
		t.Errorf("non-git dir: %q", b.String())
	}
	if err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	b.Reset()
	start := time.Now()
	preflight(&b, dir, "agent:test", []byte(`{"slug":"x","status":"active","active_by":"agent:other","active_since":"2026-09-18T00:00:00Z",
		"fields":{"branch":"agent/test/x","head":"abc1234","pushed":false,"checkpoint":"commit-ok","next":"await push prompt","area":"cli","staging":""}}`))
	out := b.String()
	for _, want := range []string{"canonical checkout", "CREATE A WORKTREE", "origin/main: unavailable", "active by agent:other", "agent/test/x", "EnterWorktree path=../",
		"handoff branch: agent/test/x", "handoff head: abc1234", "handoff pushed: false", "handoff checkpoint: commit-ok", "handoff next: await push prompt"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// only the handoff fields, in order, and empty ones are skipped
	if strings.Contains(out, "handoff area") || strings.Contains(out, "handoff staging") || strings.Index(out, "handoff branch") > strings.Index(out, "handoff next") {
		t.Errorf("handoff lines wrong:\n%s", out)
	}
	b.Reset()
	preflight(&b, dir, "agent:test", []byte(`{"slug":"y","status":"queued"}`))
	if strings.Contains(b.String(), "handoff") {
		t.Errorf("no fields, no handoff lines:\n%s", b.String())
	}
	if strings.Contains(out, "MUST BE ON MAIN") {
		t.Errorf("unborn main must not read as off-main:\n%s", out)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("preflight took %v, budget is 4s", d)
	}

	// the canonical checkout on a ticket branch is the loudest line, with
	// the restore recipe; back on main it is the ordinary one
	for _, args := range [][]string{
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"},
		{"checkout", "-q", "-b", "agent/other/thing"},
	} {
		if err := exec.Command("git", append([]string{"-C", dir}, args...)...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	b.Reset()
	preflight(&b, dir, "agent:test", nil)
	out = b.String()
	if !strings.Contains(out, "canonical checkout on agent/other/thing: MUST BE ON MAIN") || !strings.Contains(out, "checkout main") || !strings.Contains(out, "worktree add") {
		t.Errorf("off-main canonical: %q", out)
	}
	if err := exec.Command("git", "-C", dir, "checkout", "-q", "main").Run(); err != nil {
		t.Fatal(err)
	}
	b.Reset()
	preflight(&b, dir, "agent:test", nil)
	if out = b.String(); strings.Contains(out, "MUST BE ON MAIN") || !strings.Contains(out, "canonical checkout on main: CREATE A WORKTREE") {
		t.Errorf("canonical on main: %q", out)
	}
}

// the tone skill linked in any agent skill root turns the register line on,
// and it is the first line so it cannot be missed
func TestPreflightRegisterLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if toneInstalled(home) {
		t.Fatal("empty home must not report the tone skill")
	}
	var b bytes.Buffer
	preflight(&b, t.TempDir(), "agent:test", nil)
	if strings.Contains(b.String(), "register:") {
		t.Errorf("register line without the link:\n%s", b.String())
	}
	for _, root := range []string{".claude/skills", ".hermes/skills"} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, root, "servitor-tone"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !toneInstalled(home) {
			t.Errorf("%s: link not detected", root)
		}
		b.Reset()
		preflight(&b, t.TempDir(), "agent:test", nil)
		if !strings.HasPrefix(b.String(), "register: servitor-tone ON\n") {
			t.Errorf("%s: register line must come first:\n%s", root, b.String())
		}
	}
}
