package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/prestontallen/servitor/internal/testdb"
)

// TestMigrationHostKeepsLegacyRows: 003 adds ledger.host to a database that
// already holds events, rewrites none of them, and legacy rows read host NULL
// with their session untouched.
func TestMigrationHostKeepsLegacyRows(t *testing.T) {
	ctx := context.Background()
	adminDSN := testdb.AdminDSN(t)
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	defer admin.Close(ctx)
	for _, q := range []string{
		`DROP DATABASE IF EXISTS servitor_test_host WITH (FORCE)`,
		`CREATE DATABASE servitor_test_host`,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	s, err := Open(ctx, testdb.Named(t, adminDSN, "servitor_test_host"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Pool.Close()

	// the schema as it stood before 003, with events written by old code
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, migrationsTable); err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.id >= 3 {
			continue
		}
		if _, err := s.Pool.Exec(ctx, m.sql); err != nil {
			t.Fatalf("migration %03d: %v", m.id, err)
		}
		if _, err := s.Pool.Exec(ctx, `INSERT INTO schema_migrations (id, name) VALUES ($1,$2)`, m.id, m.name); err != nil {
			t.Fatal(err)
		}
	}
	ticket := NewULID()
	for _, session := range []any{"", nil} {
		if _, err := s.Pool.Exec(ctx,
			`INSERT INTO ledger (ulid, ticket_ulid, actor, actor_type, session, kind)
			 VALUES ($1,$2,'agent:cli','agent',$3,'note')`, NewULID(), ticket, session); err != nil {
			t.Fatal(err)
		}
	}
	type row struct {
		id      int64
		session *string
	}
	read := func() []row {
		rows, err := s.Pool.Query(ctx, `SELECT id, session FROM ledger ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.session); err != nil {
				t.Fatal(err)
			}
			out = append(out, r)
		}
		return out
	}
	before := read()

	if err := s.ApplySchema(ctx); err != nil {
		t.Fatalf("apply 003: %v", err)
	}
	after := read()
	if len(after) != len(before) {
		t.Fatalf("rows %d -> %d", len(before), len(after))
	}
	for i := range before {
		if after[i].id != before[i].id || (after[i].session == nil) != (before[i].session == nil) {
			t.Errorf("row %d changed: %+v -> %+v", i, before[i], after[i])
		}
	}
	var withHost int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM ledger WHERE host IS NOT NULL`).Scan(&withHost); err != nil {
		t.Fatal(err)
	}
	if withHost != 0 {
		t.Errorf("legacy rows gained a host: %d", withHost)
	}
}

// TestCtxAuthors: ctx lists a ticket's authors newest first, capped at
// AuthorsCap with the full count in authors_total; legacy rows (no host, no
// session) fold into one author per actor; system events author nothing.
func TestCtxAuthors(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)
	ticket := newTicket(t, s, "authors")
	// legacy: two rows by the default actor, one with NULL session
	for _, session := range []any{"", nil} {
		if _, err := s.Pool.Exec(ctx,
			`INSERT INTO ledger (ulid, ticket_ulid, actor, actor_type, session, kind)
			 VALUES ($1,$2,'agent:cli','agent',$3,'note')`, NewULID(), ticket, session); err != nil {
			t.Fatal(err)
		}
	}
	// eleven authors after that; the first writes twice
	for i := 0; i < 11; i++ {
		e := evt(ticket, "note", map[string]any{"v": i})
		e.Actor, e.Host, e.Session = "agent:claude", "adirondack", fmt.Sprintf("s%02d", i)
		mustAppend(t, s, e)
		if i == 0 {
			mustAppend(t, s, e)
		}
	}
	sys := evt(ticket, "note", map[string]any{"v": "hook"})
	sys.Actor, sys.ActorType = "system", "system"
	mustAppend(t, s, sys)

	raw, err := s.CtxRead(ctx, ticket)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Authors []struct {
			Actor   string  `json:"actor"`
			Host    *string `json:"host"`
			Session string  `json:"session"`
			Events  int     `json:"events"`
		} `json:"authors"`
		AuthorsTotal int `json:"authors_total"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	// newTicket's create (agent:test, session t) + legacy agent:cli + 11
	if doc.AuthorsTotal != 13 {
		t.Errorf("authors_total %d, want 13", doc.AuthorsTotal)
	}
	if len(doc.Authors) != AuthorsCap {
		t.Fatalf("authors %d, want %d", len(doc.Authors), AuthorsCap)
	}
	if a := doc.Authors[0]; a.Session != "s10" || a.Host == nil || *a.Host != "adirondack" || a.Actor != "agent:claude" {
		t.Errorf("newest author %+v", a)
	}
	for _, a := range doc.Authors {
		if a.Actor == "system" {
			t.Errorf("system listed as an author")
		}
	}

	// the legacy author, visible once the cap no longer hides it
	raw, err = s.CtxRead(ctx, newTicketWithLegacy(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var legacy int
	for _, a := range doc.Authors {
		if a.Actor == "agent:cli" {
			legacy++
			if a.Host != nil || a.Session != "" || a.Events != 2 {
				t.Errorf("legacy author %+v", a)
			}
		}
	}
	if legacy != 1 {
		t.Errorf("legacy authors %d, want 1", legacy)
	}
}

func newTicketWithLegacy(t *testing.T, s *Store) string {
	t.Helper()
	ticket := newTicket(t, s, "authors-legacy")
	for _, session := range []any{"", nil} {
		if _, err := s.Pool.Exec(context.Background(),
			`INSERT INTO ledger (ulid, ticket_ulid, actor, actor_type, session, kind)
			 VALUES ($1,$2,'agent:cli','agent',$3,'note')`, NewULID(), ticket, session); err != nil {
			t.Fatal(err)
		}
	}
	return ticket
}

// TestAppendStampsHost: a write carrying host and session stores both; a
// write with neither stores host NULL and session "" (the GUI and old-client
// path).
func TestAppendStampsHost(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)
	ticket := newTicket(t, s, "stamp")
	e := evt(ticket, "note", map[string]any{"v": "with author"})
	e.Session, e.Host = "sess-1", "adirondack"
	mustAppend(t, s, e)
	e = evt(ticket, "note", map[string]any{"v": "bare"})
	e.Session, e.Host = "", ""
	mustAppend(t, s, e)

	evs, err := s.History(ctx, ticket, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := evs[len(evs)-2:]
	if got[0].Host == nil || *got[0].Host != "adirondack" || got[0].Session == nil || *got[0].Session != "sess-1" {
		t.Errorf("stamped row: host %v session %v", got[0].Host, got[0].Session)
	}
	if got[1].Host != nil || got[1].Session == nil || *got[1].Session != "" {
		t.Errorf("bare row: host %v session %v", got[1].Host, got[1].Session)
	}
}
