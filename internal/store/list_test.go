package store

import (
	"context"
	"testing"
)

func TestListReachesDoneAndDropped(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	doneID := newTicket(t, s, "list-done")
	liveID := newTicket(t, s, "list-live")
	dropID := newTicket(t, s, "list-dropped")
	mustAppend(t, s, evt(doneID, "status.set", map[string]any{"status": "done"}))
	mustAppend(t, s, evt(dropID, "status.set", map[string]any{"status": "dropped"}))

	// AC1: a done ticket is retrievable.
	cards, err := s.List(ctx, ListFilter{Statuses: []string{"done"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].ULID != doneID {
		t.Fatalf("list done: got %v, want [%s]", cards, doneID)
	}

	// Dropped arc members are reachable too: make dropID a member.
	parent := newTicket(t, s, "list-arc")
	mustAppend(t, s, evt(dropID, "field.set", map[string]any{"field": "parent", "v": parent}))
	cards, err = s.List(ctx, ListFilter{Statuses: []string{"dropped"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].ULID != dropID {
		t.Fatalf("list dropped (arc member): got %v, want [%s]", cards, dropID)
	}

	// Empty filter = all statuses, including the live one.
	cards, err = s.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 4 { // done, live, dropped member, arc parent
		t.Fatalf("list all: got %d cards, want 4", len(cards))
	}

	_ = liveID
}

func TestListQueryCaseInsensitive(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := NewULID()
	mustAppend(t, s, evt(id, "ticket.create", map[string]any{"slug": "case-check", "title": "Mixed Case Title"}))

	for _, q := range []string{"case-check", "CASE-CHECK", "Case-Check", "mixed case", "Case Title"} {
		cards, err := s.List(ctx, ListFilter{Query: q})
		if err != nil {
			t.Fatal(err)
		}
		if len(cards) != 1 || cards[0].ULID != id {
			t.Errorf("query %q: got %v, want the ticket", q, cards)
		}
	}
	cards, err := s.List(ctx, ListFilter{Query: "no-such-thing"})
	if err != nil || len(cards) != 0 {
		t.Errorf("non-matching query: got %v err %v, want empty/nil", cards, err)
	}
}

func TestListInvalidStatus(t *testing.T) {
	s := testDB(t)
	if _, err := s.List(context.Background(), ListFilter{Statuses: []string{"done", "bogus"}}); err == nil {
		t.Error("invalid status accepted")
	}
}

func TestListLimit(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	for _, slug := range []string{"lim-a", "lim-b", "lim-c"} {
		newTicket(t, s, slug)
	}
	cards, err := s.List(ctx, ListFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("limit: got %d cards, want 2", len(cards))
	}
}
