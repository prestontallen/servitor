package store

import (
	"context"
	"testing"
	"time"
)

// note appends a note and returns its ledger id.
func note(t *testing.T, s *Store, ticket, text string) int64 {
	t.Helper()
	return mustAppend(t, s, evt(ticket, "note", map[string]any{"v": text}))
}

// tick separates event timestamps so a boundary taken between two appends
// falls strictly between their ts values.
func tick() time.Time {
	time.Sleep(20 * time.Millisecond)
	at := time.Now()
	time.Sleep(20 * time.Millisecond)
	return at
}

func ids(evs []LedgerEvent) []int64 {
	out := make([]int64, len(evs))
	for i, e := range evs {
		out[i] = e.ID
	}
	return out
}

func sameIDs(t *testing.T, name string, got []LedgerEvent, want ...int64) {
	t.Helper()
	g := ids(got)
	if len(g) != len(want) {
		t.Fatalf("%s: ids %v, want %v", name, g, want)
	}
	for i := range g {
		if g[i] != want[i] {
			t.Fatalf("%s: ids %v, want %v", name, g, want)
		}
	}
}

func TestEventsTimeWindow(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a := newTicket(t, s, "win-a")
	before := note(t, s, a, "before")
	since := tick()
	in1 := note(t, s, a, "in 1")
	in2 := note(t, s, a, "in 2")
	until := tick()
	note(t, s, a, "after")

	got, err := s.Events(ctx, LedgerFilter{Since: since, Until: until})
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, "window", got, in2, in1)

	// one edge only
	got, _ = s.Events(ctx, LedgerFilter{Until: since, Ticket: a})
	if got[0].ID != before {
		t.Errorf("until-only: newest %d, want %d", got[0].ID, before)
	}
	// [t, t) is empty: until is exclusive
	got, _ = s.Events(ctx, LedgerFilter{Since: got[0].TS, Until: got[0].TS})
	if len(got) != 0 {
		t.Errorf("empty window [t, t) returned %v", ids(got))
	}
	// since after until: empty, not an error
	got, err = s.Events(ctx, LedgerFilter{Since: until, Until: since})
	if err != nil || len(got) != 0 {
		t.Errorf("inverted window: %v, %v", ids(got), err)
	}
}

func TestEventsTicketsAndPerTicket(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a := newTicket(t, s, "multi-a")
	b := newTicket(t, s, "multi-b")
	c := newTicket(t, s, "multi-c")
	a1 := note(t, s, a, "a1")
	b1 := note(t, s, b, "b1")
	note(t, s, c, "c1")
	a2 := note(t, s, a, "a2")
	b2 := note(t, s, b, "b2")

	got, err := s.Events(ctx, LedgerFilter{Tickets: []string{a, b}, Kind: "note"})
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, "tickets a,b", got, b2, a2, b1, a1)

	got, err = s.Events(ctx, LedgerFilter{Tickets: []string{a, b}, Kind: "note", PerTicket: 1})
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, "per-ticket 1", got, b2, a2)

	// Limit still caps the total after the per-ticket cut
	got, _ = s.Events(ctx, LedgerFilter{Tickets: []string{a, b}, Kind: "note", PerTicket: 2, Limit: 3})
	sameIDs(t, "per-ticket 2 limit 3", got, b2, a2, b1)

	// unknown ULID in the set matches nothing extra
	got, _ = s.Events(ctx, LedgerFilter{Tickets: []string{a, "01NOPE"}, Kind: "note"})
	sameIDs(t, "with unknown", got, a2, a1)

	// Ticket and Tickets combine as AND
	got, _ = s.Events(ctx, LedgerFilter{Ticket: c, Tickets: []string{a, b}})
	if len(got) != 0 {
		t.Errorf("Ticket AND Tickets disjoint: %v", ids(got))
	}
}
