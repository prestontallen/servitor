package store

import (
	"context"
	"testing"
	"time"
)

// Contract (arc-schema): arcs are tickets with children via parent.
// parent is a real column (rollups query it); source/source_ref/depends/
// area stay free jsonb fields. Migration 002 adds the column + index on a
// populated store (testDB applies all migrations over a fresh DB; the
// forward-apply over pre-existing rows is covered by the baseline-stamp
// path in store_test).

func TestParentAndArcRollup(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	arc := newTicket(t, s, "arc-one")
	kidA := newTicket(t, s, "kid-a")
	kidB := newTicket(t, s, "kid-b")

	// point kids at the arc
	mustAppend(t, s, evt(kidA, "field.set", map[string]any{"field": "parent", "v": arc}))
	mustAppend(t, s, evt(kidB, "field.set", map[string]any{"field": "parent", "v": arc}))

	// all queued -> rollup queued, 2 members
	arcs, err := s.Arcs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(arcs) != 1 || arcs[0].ULID != arc {
		t.Fatalf("want 1 arc %s, got %+v", arc, arcs)
	}
	if arcs[0].Rollup != "queued" || len(arcs[0].Members) != 2 || arcs[0].LastActivity == nil {
		t.Fatalf("queued rollup wrong: %+v", arcs[0])
	}

	// kid active -> rollup active
	mustAppend(t, s, evt(kidA, "status.set", map[string]any{"status": "active"}))
	if a := mustArc(t, s, arc); a.Rollup != "active" {
		t.Fatalf("want active, got %s", a.Rollup)
	}

	// kid blocked -> rollup blocked (blocked wins over active)
	mustAppend(t, s, evt(kidB, "status.set", map[string]any{"status": "blocked", "on": "human"}))
	if a := mustArc(t, s, arc); a.Rollup != "blocked" {
		t.Fatalf("want blocked, got %s", a.Rollup)
	}

	// both done -> rollup done
	mustAppend(t, s, evt(kidA, "status.set", map[string]any{"status": "done"}))
	mustAppend(t, s, evt(kidB, "status.set", map[string]any{"status": "done"}))
	if a := mustArc(t, s, arc); a.Rollup != "done" {
		t.Fatalf("want done, got %s", a.Rollup)
	}

	// dropped members don't count
	kidC := newTicket(t, s, "kid-c")
	mustAppend(t, s, evt(kidC, "field.set", map[string]any{"field": "parent", "v": arc}))
	mustAppend(t, s, evt(kidC, "status.set", map[string]any{"status": "dropped"}))
	if a := mustArc(t, s, arc); a.Rollup != "done" || len(a.Members) != 2 {
		t.Fatalf("dropped member leaked into rollup: %+v", a)
	}

	// arc is excluded from the board; kids carry parent
	board, err := s.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range board {
		if c.ULID == arc {
			t.Errorf("arc %s leaked onto board", arc)
		}
		if c.ULID == kidA && (c.Parent == nil || *c.Parent != arc) {
			t.Errorf("kid-a board card missing parent: %+v", c)
		}
	}
}

func mustArc(t *testing.T, s *Store, ulid string) ArcSummary {
	t.Helper()
	arcs, err := s.Arcs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range arcs {
		if a.ULID == ulid {
			return a
		}
	}
	t.Fatalf("arc %s not found in %+v", ulid, arcs)
	return ArcSummary{}
}

func TestParentGuards(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	arc := newTicket(t, s, "arc-guard")
	kid := newTicket(t, s, "kid-guard")

	// unknown parent rejected
	_, err := s.AppendEvent(ctx, evt(kid, "field.set", map[string]any{"field": "parent", "v": NewULID()}), time.Time{})
	expectFail(t, "unknown parent", err, "unknown parent ticket")

	// self-parent rejected
	_, err = s.AppendEvent(ctx, evt(arc, "field.set", map[string]any{"field": "parent", "v": arc}), time.Time{})
	expectFail(t, "self parent", err, "its own parent")

	// cycle rejected: kid under arc, then arc under kid
	mustAppend(t, s, evt(kid, "field.set", map[string]any{"field": "parent", "v": arc}))
	_, err = s.AppendEvent(ctx, evt(arc, "field.set", map[string]any{"field": "parent", "v": kid}), time.Time{})
	expectFail(t, "cycle", err, "would create a cycle")

	// absent v clears parent
	mustAppend(t, s, evt(kid, "field.set", map[string]any{"field": "parent"}))
	board, err := s.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// arc has no children now -> back on the board as a plain ticket
	found := false
	for _, c := range board {
		if c.ULID == arc {
			found = true
		}
	}
	if !found {
		t.Errorf("cleared arc should return to the board")
	}
}

func TestEventClassMap(t *testing.T) {
	// contract: every known kind classified; unknown kinds are transitions
	signals := map[string]bool{"note": true, "decision": true, "gate": true, "feedback": true}
	for _, k := range []string{"ticket.create", "status.set", "field.set",
		"subitem.add", "subitem.set", "subitem.rank", "hook", "made_up_kind"} {
		if EventClass(k) != "transition" {
			t.Errorf("%s: want transition", k)
		}
	}
	for k := range signals {
		if EventClass(k) != "signal" {
			t.Errorf("%s: want signal", k)
		}
	}
}

func TestHistoryCarriesClass(t *testing.T) {
	s := testDB(t)
	tk := newTicket(t, s, "class-ticket")
	mustAppend(t, s, evt(tk, "note", map[string]any{"v": "a signal"}))
	mustAppend(t, s, evt(tk, "status.set", map[string]any{"status": "active"}))

	evs, err := s.History(context.Background(), tk, 100)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string]string{}
	for _, e := range evs {
		byKind[e.Kind] = e.Class
	}
	if byKind["ticket.create"] != "transition" {
		t.Errorf("ticket.create: want transition, got %q", byKind["ticket.create"])
	}
	if byKind["note"] != "signal" {
		t.Errorf("note: want signal, got %q", byKind["note"])
	}
	if byKind["status.set"] != "transition" {
		t.Errorf("status.set: want transition, got %q", byKind["status.set"])
	}
}

func TestGlobalEventsFiltered(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	a := newTicket(t, s, "evt-a")
	b := newTicket(t, s, "evt-b")
	mustAppend(t, s, evt(a, "note", map[string]any{"v": "note on a"}))
	mustAppend(t, s, evt(b, "status.set", map[string]any{"status": "active"}))
	mustAppend(t, s, evt(a, "note", map[string]any{"v": "second note on a"}))

	// unfiltered: newest first, class set
	all, err := s.Events(ctx, LedgerFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 5 {
		t.Fatalf("want >= 5 events, got %d", len(all))
	}
	if all[0].ID < all[len(all)-1].ID {
		t.Errorf("want newest-first ordering")
	}
	for _, e := range all {
		if e.Class != "signal" && e.Class != "transition" {
			t.Errorf("event %d: bad class %q", e.ID, e.Class)
		}
	}

	// kind filter
	notes, err := s.Events(ctx, LedgerFilter{Kind: "note"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range notes {
		if e.Kind != "note" {
			t.Errorf("kind filter leaked %s", e.Kind)
		}
		if e.Class != "signal" {
			t.Errorf("note should be signal, got %s", e.Class)
		}
	}

	// ticket filter (exact ULID)
	onA, err := s.Events(ctx, LedgerFilter{Ticket: a})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range onA {
		if e.Ticket != a {
			t.Errorf("ticket filter leaked %s", e.Ticket)
		}
	}

	// actor_type filter
	human, err := s.Events(ctx, LedgerFilter{ActorType: "human"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range human {
		if e.ActorType != "human" {
			t.Errorf("actor_type filter leaked %s", e.ActorType)
		}
	}

	// keyset: since_id = max of first page skips it
	if all[0].ID == all[len(all)-1].ID {
		t.Skip("single-event page; keyset untestable")
	}
	page2, err := s.Events(ctx, LedgerFilter{SinceID: all[0].ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range page2 {
		if e.ID >= all[0].ID {
			t.Errorf("since_id filter leaked id %d", e.ID)
		}
	}
}
