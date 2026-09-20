package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prestontallen/servitor/internal/store"
)

func tlEvents(t *testing.T, s Service, ulid, slug string) {
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
	for _, c := range steps {
		if _, err := s.Append(ctx, c); err != nil {
			t.Fatalf("%s: %v", c.Kind, err)
		}
	}
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
	tlEvents(t, s, id, "timeline-clip")

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

	// a window starting mid-life clips segment starts to its since edge
	since := time.Now().Add(-2 * time.Hour)
	tl, err = s.Timeline(ctx, TimelineQuery{Since: &since})
	if err != nil {
		t.Fatal(err)
	}
	for _, seg := range tl.Segments {
		if seg.Ticket != id {
			continue
		}
		if seg.From.Before(since) {
			t.Errorf("segment starts before the window: %+v", seg)
		}
	}
	if got := phases(tl, id); len(got) == 0 {
		t.Error("current window lost the ticket's segments")
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
}
