package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prestontallen/servitor/internal/store"
)

func TestAnalyticsShape(t *testing.T) {
	s := svc(t)
	ctx := context.Background()
	id := store.NewULID()
	if _, err := s.Append(ctx, WriteCmd{Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": "analytics-test", "title": "A"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(ctx, WriteCmd{Ticket: id, Kind: "note", Actor: "agent:test",
		Payload: map[string]any{"v": "one"}}); err != nil {
		t.Fatal(err)
	}

	buckets, err := s.Analytics(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	var found *DayBucket
	for i := range buckets {
		if buckets[i].Day == today {
			found = &buckets[i]
		}
	}
	if found == nil {
		t.Fatalf("no bucket for today: %+v", buckets)
	}
	if found.Events != 2 {
		t.Errorf("events=%d want 2", found.Events)
	}
	if found.ByKind["ticket.create"] != 1 || found.ByKind["note"] != 1 {
		t.Errorf("by_kind=%v", found.ByKind)
	}

	// endpoint serves the same shape
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/analytics?days=30", nil)
	NewHTTP(s).Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"by_kind"`) {
		t.Error("analytics endpoint missing by_kind")
	}
	_ = json.RawMessage{}
}
