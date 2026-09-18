package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSubitemSetResolvesPrefix(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "set-prefix-test")
	a := addSubitem(t, s, id, "criterion", "alpha")
	addSubitem(t, s, id, "criterion", "beta")

	// an 8-char prefix updates exactly one subitem
	if _, err := s.AppendEvent(ctx, evt(id, "subitem.set",
		map[string]any{"ulid": a[:16], "body": "alpha v2"}), time.Time{}); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := s.Pool.QueryRow(ctx, `SELECT body FROM subitems WHERE ulid=$1`, a).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "alpha v2" {
		t.Errorf("body after set: %q", body)
	}

	// unknown and ambiguous prefixes fail
	if _, err := s.AppendEvent(ctx, evt(id, "subitem.set",
		map[string]any{"ulid": "ZZZZZZ", "body": "x"}), time.Time{}); err == nil {
		t.Error("unknown prefix accepted")
	}
	if _, err := s.AppendEvent(ctx, evt(id, "subitem.set",
		map[string]any{"ulid": a[:1], "body": "x"}), time.Time{}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("want ambiguity error, got %v", err)
	}
}
