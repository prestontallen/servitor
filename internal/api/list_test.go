package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/prestontallen/servitor/internal/store"
)

// TestListEndpointReachesDropped covers the archive contract: done and
// dropped tickets — including dropped arc members — are retrievable via
// GET /api/tickets, and invalid status is a 422, never an empty list.
func TestListEndpointReachesDropped(t *testing.T) {
	s := svc(t)
	ctx := context.Background()

	done := mustCreate(t, s, "api-list-done")
	if _, err := s.Append(ctx, WriteCmd{Ticket: done, Kind: "status.set", Actor: "agent:test", Payload: map[string]any{"status": "done"}}); err != nil {
		t.Fatal(err)
	}
	dropped := mustCreate(t, s, "api-list-dropped")
	if _, err := s.Append(ctx, WriteCmd{Ticket: dropped, Kind: "status.set", Actor: "agent:test", Payload: map[string]any{"status": "dropped"}}); err != nil {
		t.Fatal(err)
	}

	get := func(path string) (int, []store.Card) {
		rec := httptest.NewRecorder()
		NewHTTP(s).Routes().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		var cards []store.Card
		json.Unmarshal(rec.Body.Bytes(), &cards)
		return rec.Code, cards
	}

	// done is retrievable
	code, cards := get("/api/tickets?status=done")
	if code != 200 || len(cards) != 1 || cards[0].ULID != done {
		t.Fatalf("status=done: code %d cards %v", code, cards)
	}
	// dropped (arc member) is retrievable
	code, cards = get("/api/tickets?status=dropped")
	if code != 200 || len(cards) != 1 || cards[0].ULID != dropped {
		t.Fatalf("status=dropped: code %d cards %v", code, cards)
	}
	// query is case-insensitive on slug
	code, cards = get("/api/tickets?q=API-LIST-DONE")
	if code != 200 || len(cards) != 1 || cards[0].ULID != done {
		t.Fatalf("q case-insensitive: code %d cards %v", code, cards)
	}
	// invalid status -> 422 invalid_status
	rec := httptest.NewRecorder()
	NewHTTP(s).Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/api/tickets?status=bogus", nil))
	if rec.Code != 422 {
		t.Fatalf("invalid status: want 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var er struct {
		Error struct{ Code string }
	}
	json.Unmarshal(rec.Body.Bytes(), &er)
	if er.Error.Code != "invalid_status" {
		t.Errorf("code %q, want invalid_status", er.Error.Code)
	}
}
