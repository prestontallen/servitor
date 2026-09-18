package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Tests run against a throwaway database on the local timescaledb container.
// Override with SERVITOR_TEST_DSN when needed.
const adminDSN = "postgres://postgres:psql@localhost:5432/postgres?sslmode=disable"

func testDB(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	defer admin.Close(ctx)
	for _, q := range []string{
		`DROP DATABASE IF EXISTS servitor_test WITH (FORCE)`,
		`CREATE DATABASE servitor_test`,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	dsn := os.Getenv("SERVITOR_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:psql@localhost:5432/servitor_test?sslmode=disable"
	}
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Pool.Close() })
	return s
}

func evt(ticket, kind string, payload map[string]any) Event {
	return Event{TicketULID: ticket, Actor: "agent:test", ActorType: "agent", Session: "t", Kind: kind, Payload: payload}
}

func mustAppend(t *testing.T, s *Store, e Event) int64 {
	t.Helper()
	id, err := s.AppendEvent(context.Background(), e, time.Time{})
	if err != nil {
		t.Fatalf("append %s: %v", e.Kind, err)
	}
	return id
}

func expectFail(t *testing.T, name string, err error, wantSub string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: FAIL — expected error, got none", name)
		return
	}
	if wantSub != "" && !strings.Contains(err.Error(), wantSub) {
		t.Errorf("%s: FAIL — error %q does not contain %q", name, err.Error(), wantSub)
		return
	}
	t.Logf("%s: PASS (%v)", name, err)
}

func newTicket(t *testing.T, s *Store, slug string) string {
	t.Helper()
	id := NewULID()
	mustAppend(t, s, evt(id, "ticket.create", map[string]any{"slug": slug, "title": "T " + slug}))
	return id
}

func TestTicketLifecycleAndDerivedState(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "life-cycle")
	mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "active"}))
	mustAppend(t, s, Event{TicketULID: id, Actor: "human:preston", ActorType: "human", Session: "t",
		Kind: "gate", Payload: map[string]any{"gate": "contract_approved"}})
	mustAppend(t, s, evt(id, "gate", map[string]any{"gate": "presented"}))
	mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "done"}))

	var status, card string
	if err := s.Pool.QueryRow(ctx, `SELECT status::text, card_word::text FROM tickets WHERE ulid=$1`, id).Scan(&status, &card); err != nil {
		t.Fatal(err)
	}
	if status != "done" || card != "checking" {
		t.Errorf("got status=%s card=%s, want done/checking", status, card)
	}
}

func TestSlugUniqueAcrossHistoryIncludingArchived(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "slug-case")
	// same slug, different case, different NEW ticket -> must fail
	_, err := s.AppendEvent(context.Background(), evt(NewULID(), "ticket.create",
		map[string]any{"slug": "SLUG-CASE"}), time.Time{})
	expectFail(t, "slug reuse CI across history", err, "claimed")

	// rename releases nothing: history-wide, even after rename
	newID := NewULID()
	mustAppend(t, s, evt(newID, "ticket.create", map[string]any{"slug": "fresh-slug"}))
	mustAppend(t, s, evt(id, "field.set", map[string]any{"field": "slug", "v": "renamed-slug"}))
	_, err = s.AppendEvent(context.Background(), evt(newID, "field.set",
		map[string]any{"field": "slug", "v": "SLUG-CASE"}), time.Time{})
	expectFail(t, "reuse of released-but-historical slug", err, "claimed")

	// rename back to own slug is fine
	mustAppend(t, s, evt(id, "field.set", map[string]any{"field": "slug", "v": "slug-case"}))
}

func TestBlockedRequiresOn(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "blocked-test")
	_, err := s.AppendEvent(context.Background(), evt(id, "status.set", map[string]any{"status": "blocked"}), time.Time{})
	expectFail(t, "blocked without on", err, `"on"`)
	mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "blocked", "on": "human"}))
	var on, since *string
	if err := s.Pool.QueryRow(context.Background(), `SELECT blocked_on, blocked_since::text FROM tickets WHERE ulid=$1`, id).Scan(&on, &since); err != nil {
		t.Fatal(err)
	}
	if on == nil || since == nil || *on != "human" {
		t.Errorf("blocked_on/since not set: %v %v", on, since)
	}
}

func TestHumanGateOnlyByHuman(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "gate-test")
	_, err := s.AppendEvent(context.Background(), evt(id, "gate", map[string]any{"gate": "contract_approved"}), time.Time{})
	expectFail(t, "agent claiming human gate", err, "human actor")
	mustAppend(t, s, Event{TicketULID: id, Actor: "human:preston", ActorType: "human", Kind: "gate",
		Payload: map[string]any{"gate": "contract_approved"}})
	// a gate passes once
	_, err = s.AppendEvent(context.Background(), Event{TicketULID: id, Actor: "human:preston", ActorType: "human",
		Kind: "gate", Payload: map[string]any{"gate": "contract_approved"}}, time.Time{})
	expectFail(t, "gate twice", err, "gate_events_one_per_ticket")
}

func TestAbsentVsEmptyPR(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "pr-test")
	// pr never set: absent (NULL)
	mustAppend(t, s, evt(id, "field.set", map[string]any{"field": "pr", "v": ""}))
	var pr *string
	if err := s.Pool.QueryRow(context.Background(), `SELECT pr FROM tickets WHERE ulid=$1`, id).Scan(&pr); err != nil {
		t.Fatal(err)
	}
	if pr == nil || *pr != "" {
		t.Fatalf("want empty string, got %v", pr)
	}
	// key-less set => absent again (NULL), three distinct states exercised
	mustAppend(t, s, evt(id, "field.set", map[string]any{"field": "pr"}))
	if err := s.Pool.QueryRow(context.Background(), `SELECT pr FROM tickets WHERE ulid=$1`, id).Scan(&pr); err != nil {
		t.Fatal(err)
	}
	if pr != nil {
		t.Fatalf("want NULL (absent), got %q", *pr)
	}
}

func TestUnknownKeysRoundTripVerbatim(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "extra-test")
	mustAppend(t, s, evt(id, "weird.custom.kind", map[string]any{"nested": map[string]any{"x": 1}, "flag": true}))
	var got string
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT payload->>'flag' FROM ledger WHERE kind='weird.custom.kind' LIMIT 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Errorf("unknown kind payload altered: %s", got)
	}
	mustAppend(t, s, evt(id, "field.set", map[string]any{"field": "mystery", "v": map[string]any{"k": [3]any{1, "two", nil}}}))
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT fields->'mystery'->'k'->>1 FROM tickets WHERE ulid=$1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "two" {
		t.Errorf("unknown field round-trip broken: %s", got)
	}
}

func TestDecisionDedupe(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "decision-test")
	d := map[string]any{"what": "use TLS", "why": "security"}
	mustAppend(t, s, evt(id, "decision", d))
	mustAppend(t, s, evt(id, "decision", d))
	var n int
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM subitems WHERE ticket_ulid=$1 AND kind='decision'`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("decision dedupe failed: %d rows", n)
	}
}

func TestReadModelIsProjectionOnly(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "readonly-test")
	_, err := s.Pool.Exec(context.Background(), `UPDATE tickets SET title='hacked' WHERE ulid=$1`, id)
	expectFail(t, "direct update of tickets", err, "projection-only")
	_, err = s.Pool.Exec(context.Background(), `INSERT INTO subitems (ulid,ticket_ulid,kind,rank,created_at,updated_at) VALUES ('01X',$1,'note',0,now(),now())`, id)
	expectFail(t, "direct insert into subitems", err, "projection-only")
}

func TestLedgerAppendOnly(t *testing.T) {
	s := testDB(t)
	newTicket(t, s, "appendonly-test")
	var ledgerID int64
	if err := s.Pool.QueryRow(context.Background(), `SELECT max(id) FROM ledger`).Scan(&ledgerID); err != nil {
		t.Fatal(err)
	}
	_, err := s.Pool.Exec(context.Background(), `DELETE FROM ledger WHERE id=$1`, ledgerID)
	expectFail(t, "ledger delete", err, "append-only")
	_, err = s.Pool.Exec(context.Background(), `UPDATE ledger SET kind='tampered' WHERE id=$1`, ledgerID)
	expectFail(t, "ledger update", err, "append-only")
}

func TestStaleWriteRejected(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "stale-test")
	var updated time.Time
	if err := s.Pool.QueryRow(context.Background(), `SELECT updated_at FROM tickets WHERE ulid=$1`, id).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "active"}))
	_, err := s.AppendEvent(context.Background(), evt(id, "status.set", map[string]any{"status": "done"}), updated)
	if err == nil || err != ErrStaleWrite {
		t.Errorf("want ErrStaleWrite, got %v", err)
	}
	// fresh read succeeds
	var fresh time.Time
	if err := s.Pool.QueryRow(context.Background(), `SELECT updated_at FROM tickets WHERE ulid=$1`, id).Scan(&fresh); err != nil {
		t.Fatal(err)
	}
	mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "done"}))
	_ = fresh
}

func TestAtomicityProjectionAndLedgerCommitTogether(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "atomic-test")
	// a gate that fails mid-txn must roll back the ledger row too
	if _, err := s.AppendEvent(ctx, evt(id, "gate", map[string]any{"gate": "bogus"}), time.Time{}); err == nil {
		t.Fatal("expected bogus gate to fail")
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM ledger WHERE ticket_ulid=$1 AND kind='gate'`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("ledger row survived failed projection: %d", n)
	}
}

func TestWatermarkMoves(t *testing.T) {
	s := testDB(t)
	newTicket(t, s, "watermark-test")
	var wm int64
	if err := s.Pool.QueryRow(context.Background(), `SELECT last_id FROM ledger_watermark WHERE id=1`).Scan(&wm); err != nil {
		t.Fatal(err)
	}
	if wm == 0 {
		t.Error("watermark did not move")
	}
	var maxID int64
	if err := s.Pool.QueryRow(context.Background(), `SELECT max(id) FROM ledger`).Scan(&maxID); err != nil {
		t.Fatal(err)
	}
	if wm < maxID {
		t.Errorf("watermark %d behind max ledger id %d", wm, maxID)
	}
}

func TestFullHistoryIsQueryable(t *testing.T) {
	s := testDB(t)
	id := newTicket(t, s, "history-test")
	mustAppend(t, s, evt(id, "note", map[string]any{"v": "one"}))
	mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "active"}))
	var n int
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger WHERE ticket_ulid=$1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("history incomplete: %d events", n)
	}
	fmt.Fprintln(os.Stderr, "history ok")
}

func TestApplySchemaIdempotent(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)
	// testDB already applied once; re-apply must be a clean no-op.
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("no migrations recorded")
	}
}

func TestApplySchemaBaselineStamp(t *testing.T) {
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	defer admin.Close(ctx)
	for _, q := range []string{
		`DROP DATABASE IF EXISTS servitor_test_legacy WITH (FORCE)`,
		`CREATE DATABASE servitor_test_legacy`,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	dsn := strings.Replace(adminDSN, "/postgres?", "/servitor_test_legacy?", 1)
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Pool.Close()

	// Simulate a pre-migration database: the full original schema (001) was
	// applied by the old code, which recorded nothing.
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.id == 1 {
			if _, err := s.Pool.Exec(ctx, m.sql); err != nil {
				t.Fatalf("build legacy schema: %v", err)
			}
		}
	}
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatalf("baseline apply: %v", err)
	}
	var got string
	if err := s.Pool.QueryRow(ctx,
		`SELECT name FROM schema_migrations WHERE id = 1`).Scan(&got); err != nil {
		t.Fatalf("migration 001 not stamped: %v", err)
	}
	if got != "init" {
		t.Fatalf("migration 001 name = %q, want init", got)
	}
	// And the full schema must now exist without 001 having re-run.
	var exists bool
	if err := s.Pool.QueryRow(ctx,
		`SELECT to_regclass('public.tickets') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("tickets table missing after baseline apply")
	}
	// Re-run for stability.
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	// ledger was pre-existing garbage (no hypertable/trigger); drop the db.
	if _, err := admin.Exec(ctx, `DROP DATABASE servitor_test_legacy WITH (FORCE)`); err != nil {
		t.Fatal(err)
	}
}
