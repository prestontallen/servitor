package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAppendWithoutAuthor: a raw POST with no host and no session (the GUI
// and old-client path) is accepted and reads back host null, session "";
// the headers the CLI sends land on the row.
func TestAppendWithoutAuthor(t *testing.T) {
	s := svc(t)
	ctx := context.Background()
	id := mustCreate(t, s, "api-author")
	post := func(hdr map[string]string) {
		body := `{"ticket":"` + id + `","kind":"note","actor":"agent:test","payload":{"v":"x"}}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		NewHTTP(s).Routes().ServeHTTP(rec, req)
		if rec.Code != 201 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
	}
	post(nil)
	post(map[string]string{"X-Servitor-Host": "adirondack", "X-Servitor-Session": "S"})
	evs, err := s.History(ctx, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	bare, stamped := evs[len(evs)-2], evs[len(evs)-1]
	if bare.Host != nil || bare.Session == nil || *bare.Session != "" {
		t.Errorf("bare: host %v session %v", bare.Host, bare.Session)
	}
	if stamped.Host == nil || *stamped.Host != "adirondack" || stamped.Session == nil || *stamped.Session != "S" {
		t.Errorf("stamped: host %v session %v", stamped.Host, stamped.Session)
	}
}
