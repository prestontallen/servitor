package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func addSubitem(t *testing.T, s *Store, ticket, kind, body string) string {
	t.Helper()
	id := NewULID()
	mustAppend(t, s, evt(ticket, "subitem.add", map[string]any{"ulid": id, "kind": kind, "rank": 0, "body": body}))
	return id
}

func TestSubitemPrefixResolution(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "prefix-test")
	a := addSubitem(t, s, id, "criterion", "alpha")
	addSubitem(t, s, id, "criterion", "beta")

	var got string
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var e error
		got, e = s.ResolveSubitem(ctx, tx, id, a)
		return e
	})
	if err != nil || got != a {
		t.Errorf("prefix resolve: got %s err %v, want %s", got, err, a)
	}

	// an 8-char prefix of two same-millisecond ULIDs is correctly ambiguous
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		_, e := s.ResolveSubitem(ctx, tx, id, a[:8])
		return e
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("want ambiguity error, got %v", err)
	}

	// empty prefix
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		_, e := s.ResolveSubitem(ctx, tx, id, "")
		return e
	})
	if err == nil {
		t.Error("empty prefix accepted")
	}
}

func TestRankReorderByPrefix(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "rank-test")
	a := addSubitem(t, s, id, "plan", "step a")
	b := addSubitem(t, s, id, "plan", "step b")
	mustAppend(t, s, evt(id, "subitem.rank", map[string]any{"ulid": a[:10], "rank": 7}))

	var ra, rb int64
	if err := s.Pool.QueryRow(ctx, `SELECT rank FROM subitems WHERE ulid=$1`, a).Scan(&ra); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT rank FROM subitems WHERE ulid=$1`, b).Scan(&rb); err != nil {
		t.Fatal(err)
	}
	if ra != 7 || rb != 0 {
		t.Errorf("ranks after reorder: a=%d b=%d, want a=7 b=0", ra, rb)
	}

	// ambiguous prefix must fail and roll back
	if _, err := s.AppendEvent(ctx, evt(id, "subitem.rank", map[string]any{"ulid": a[:1], "rank": 1}), time.Time{}); err == nil {
		t.Error("ambiguous prefix accepted")
	}
	// unknown prefix must fail
	if _, err := s.AppendEvent(ctx, evt(id, "subitem.rank", map[string]any{"ulid": "ZZZZZZ", "rank": 1}), time.Time{}); err == nil {
		t.Error("unknown prefix accepted")
	}
}

func TestCtxReadWholeAggregate(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "ctx-read-test")
	mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "active"}))
	mustAppend(t, s, Event{TicketULID: id, Actor: "human:preston", ActorType: "human", Kind: "gate",
		Payload: map[string]any{"gate": "contract_approved"}})
	addSubitem(t, s, id, "criterion", "works")
	mustAppend(t, s, evt(id, "decision", map[string]any{"what": "use Go", "why": "type safety"}))
	mustAppend(t, s, evt(id, "note", map[string]any{"v": "a note"}))
	mustAppend(t, s, evt(id, "field.set", map[string]any{"field": "priority", "v": "high"}))

	// by ULID
	doc, err := s.CtxRead(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		t.Fatal(err)
	}
	if m["slug"] != "ctx-read-test" || m["status"] != "active" || m["card_word"] != "building" {
		t.Errorf("ctx basics wrong: slug=%v status=%v card=%v", m["slug"], m["status"], m["card_word"])
	}
	if len(m["gates"].([]any)) != 1 || len(m["criteria"].([]any)) != 1 ||
		len(m["decisions"].([]any)) != 1 || len(m["notes"].([]any)) != 1 {
		t.Errorf("aggregate sections wrong: %+v", m)
	}
	if m["fields"].(map[string]any)["priority"] != "high" {
		t.Errorf("fields missing: %v", m["fields"])
	}
	if m["head"].(float64) <= 0 {
		t.Errorf("head not exposed: %v", m["head"])
	}

	// by slug, case-insensitive
	doc2, err := s.CtxRead(ctx, "CTX-READ-TEST")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc2), "ctx-read-test") {
		t.Error("slug lookup failed")
	}

	// unknown ticket errors
	if _, err := s.CtxRead(ctx, "01AAAAAAAAAAAAAAAAAAAAAAAA"); err == nil {
		t.Error("ctx-read of unknown ticket succeeded")
	}
}
