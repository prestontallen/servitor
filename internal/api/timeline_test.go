package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prestontallen/servitor/internal/store"
)

func tlEvents(t *testing.T, s Service, ulid, slug string) time.Time {
	t.Helper()
	ctx := context.Background()
	steps := []WriteCmd{
		{Ticket: ulid, Kind: "ticket.create", Actor: "agent:test", Payload: map[string]any{"slug": slug, "title": "T"}},
		{Ticket: ulid, Kind: "status.set", Actor: "agent:test", Payload: map[string]any{"status": "active"}},
		{Ticket: ulid, Kind: "gate", Actor: "human:preston", Payload: map[string]any{"gate": "contract_approved"}},
		{Ticket: ulid, Kind: "status.set", Actor: "agent:test", Payload: map[string]any{"status": "blocked", "on": "human"}},
		{Ticket: ulid, Kind: "status.set", Actor: "agent:test", Payload: map[string]any{"status": "active"}},
		{Ticket: ulid, Kind: "gate", Actor: "agent:test", Payload: map[string]any{"gate": "presented"}},
	}
	var mid time.Time // between the active step and the first gate
	for i, c := range steps {
		if _, err := s.Append(ctx, c); err != nil {
			t.Fatalf("%s: %v", c.Kind, err)
		}
		if i == 1 {
			mid = time.Now()
			time.Sleep(30 * time.Millisecond) // give the next event a later ts
		}
	}
	return mid
}

func phases(tl Timeline, ulid string) []string {
	var out []string
	for _, seg := range tl.Segments {
		if seg.Ticket == ulid {
			out = append(out, seg.Phase)
		}
	}
	return out
}

func TestTimelineSegmentReplay(t *testing.T) {
	s := svc(t)
	ctx := context.Background()
	id := store.NewULID()
	tlEvents(t, s, id, "timeline-replay")
	tl, err := s.Timeline(ctx, TimelineQuery{Days: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := phases(tl, id)
	want := []string{"queued", "shaping", "building", "blocked", "building", "checking"}
	if len(got) != len(want) {
		t.Fatalf("phases=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phases=%v want %v", got, want)
		}
	}
	// the slug rides along and boundaries are ordered
	for _, seg := range tl.Segments {
		if seg.Ticket == id && (seg.Slug != "timeline-replay" || seg.To.Before(seg.From)) {
			t.Errorf("bad segment %+v", seg)
		}
	}
}

func TestTimelineWindowClipping(t *testing.T) {
	s := svc(t)
	ctx := context.Background()
	id := store.NewULID()
	mid := tlEvents(t, s, id, "timeline-clip")

	// a window entirely in the past holds nothing of this ticket
	pastSince := time.Now().Add(-48 * time.Hour)
	pastUntil := time.Now().Add(-24 * time.Hour)
	tl, err := s.Timeline(ctx, TimelineQuery{Since: &pastSince, Until: &pastUntil})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(phases(tl, id)); n != 0 {
		t.Errorf("past window returned %d segments, want 0", n)
	}

	// since lands mid-life, after the activation and before the first
	// gate: the shaping segment straddles it and must clip to the edge
	tl, err = s.Timeline(ctx, TimelineQuery{Since: &mid})
	if err != nil {
		t.Fatal(err)
	}
	var mine []Segment
	for _, seg := range tl.Segments {
		if seg.Ticket == id {
			mine = append(mine, seg)
		}
	}
	if len(mine) == 0 {
		t.Fatal("current window lost the ticket's segments")
	}
	if !mine[0].From.Equal(mid) || mine[0].Phase != "shaping" {
		t.Errorf("first segment %+v, want from==since, phase shaping (the clip branch)", mine[0])
	}
	for _, seg := range mine {
		if seg.From.Before(mid) {
			t.Errorf("segment starts before the window: %+v", seg)
		}
	}
}

func TestTimelineGateWhileQueued(t *testing.T) {
	s := svc(t)
	ctx := context.Background()
	id := store.NewULID()
	steps := []WriteCmd{
		{Ticket: id, Kind: "ticket.create", Actor: "agent:test", Payload: map[string]any{"slug": "timeline-queued-gate", "title": "Q"}},
		{Ticket: id, Kind: "gate", Actor: "human:preston", Payload: map[string]any{"gate": "contract_approved"}},
		{Ticket: id, Kind: "status.set", Actor: "agent:test", Payload: map[string]any{"status": "active"}},
	}
	for _, c := range steps {
		if _, err := s.Append(ctx, c); err != nil {
			t.Fatalf("%s: %v", c.Kind, err)
		}
	}
	tl, err := s.Timeline(ctx, TimelineQuery{Days: 1})
	if err != nil {
		t.Fatal(err)
	}
	// the board keeps a queued card in the queued lane regardless of
	// card_word; the segments must agree and not draw two hours of
	// building before the ticket was ever active
	got := phases(tl, id)
	want := []string{"queued", "building"}
	if len(got) != len(want) {
		t.Fatalf("phases=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phases=%v want %v", got, want)
		}
	}
}

func TestTimelineEndpoint(t *testing.T) {
	s := svc(t)
	id := store.NewULID()
	tlEvents(t, s, id, "timeline-endpoint")

	rr := httptest.NewRecorder()
	NewHTTP(s).Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/api/timeline?days=7", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var tl Timeline
	if err := json.Unmarshal(rr.Body.Bytes(), &tl); err != nil {
		t.Fatal(err)
	}
	if len(tl.Segments) == 0 || len(tl.Buckets) == 0 {
		t.Fatalf("segments=%d buckets=%d, both must be non-empty on a used store", len(tl.Segments), len(tl.Buckets))
	}
	for _, b := range tl.Buckets {
		for actor := range b.ByActor {
			if actor != "agent" && actor != "human" && actor != "system" {
				t.Errorf("unexpected actor_type %q", actor)
			}
		}
	}

	// bad timestamps are 422, and until<=since is 422
	rr = httptest.NewRecorder()
	NewHTTP(s).Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/api/timeline?since=garbage", nil))
	if rr.Code != 422 {
		t.Errorf("bad since: status %d, want 422", rr.Code)
	}
	rr = httptest.NewRecorder()
	NewHTTP(s).Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/api/timeline?until=2026-01-01&since=2026-02-01", nil))
	if rr.Code != 422 {
		t.Errorf("until<=since: status %d, want 422", rr.Code)
	}
	// until alone derives since from until — a past until is a valid
	// short window, not since>until
	rr = httptest.NewRecorder()
	NewHTTP(s).Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/api/timeline?until=2020-01-01", nil))
	if rr.Code != 200 {
		t.Errorf("until alone: status %d, want 200", rr.Code)
	}
	// non-integer and non-positive days are 422, matching the contract
	for _, bad := range []string{"days=abc", "days=0", "days=-5"} {
		rr = httptest.NewRecorder()
		NewHTTP(s).Routes().ServeHTTP(rr, httptest.NewRequest("GET", "/api/timeline?"+bad, nil))
		if rr.Code != 422 {
			t.Errorf("%s: status %d, want 422", bad, rr.Code)
		}
	}
}
