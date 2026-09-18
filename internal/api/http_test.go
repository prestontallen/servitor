package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prestontallen/servitor/internal/store"
)

func svc(t *testing.T) Service {
	st := storeForTest(t)
	return NewStoreService(st)
}

func TestCtxEndpointRawJSON(t *testing.T) {
	s := svc(t)
	id := mustCreate(t, s, "api-ctx")

	req := httptest.NewRequest("GET", "/api/ticket/api-ctx", nil)
	rec := httptest.NewRecorder()
	NewHTTP(s).Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	// 1a: raw resource JSON — the document itself, no envelope
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["slug"] != "api-ctx" {
		t.Errorf("got %v", doc)
	}
	// additive keys: absent-able fields are null, never missing
	for _, k := range []string{"pr", "card_word", "blocked_on", "fields", "gates", "head"} {
		if _, ok := doc[k]; !ok {
			t.Errorf("key %q missing from ctx doc", k)
		}
	}
	_ = id
}

func TestAppendAndErrorCode(t *testing.T) {
	s := svc(t)
	ctx := context.Background()
	id := mustCreate(t, s, "api-append")

	// agent cannot claim the human gate -> 422 human_gate_required
	body := `{"ticket":"` + id + `","kind":"gate","actor":"agent:test","payload":{"gate":"contract_approved"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	NewHTTP(s).Routes().ServeHTTP(rec, req)
	if rec.Code != 422 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var er struct {
		Error struct{ Code string }
	}
	json.Unmarshal(rec.Body.Bytes(), &er)
	if er.Error.Code != "human_gate_required" {
		t.Errorf("code %q", er.Error.Code)
	}

	// human gate via header attestation -> 201
	body = `{"ticket":"api-append","kind":"gate","actor":"agent:test","payload":{"gate":"contract_approved"}}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	req.Header.Set("X-Servitor-Actor", "human:preston")
	NewHTTP(s).Routes().ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	// stale write -> 409 stale_write
	var updated time.Time
	if err := s.(*StoreService).Store.Pool.QueryRow(ctx,
		`SELECT updated_at FROM tickets WHERE slug='api-append'`).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	stale := updated.Add(time.Second)
	body = `{"ticket":"api-append","kind":"status.set","actor":"agent:test","payload":{"status":"active"},"expect_updated":"` + stale.Format(time.RFC3339Nano) + `"}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	NewHTTP(s).Routes().ServeHTTP(rec, req)
	if rec.Code != 409 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &er)
	if er.Error.Code != "stale_write" {
		t.Errorf("code %q", er.Error.Code)
	}
}

func TestBoardEndpoint(t *testing.T) {
	s := svc(t)
	mustCreate(t, s, "board-one")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/board", nil)
	NewHTTP(s).Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var cards []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cards); err != nil {
		t.Fatal(err)
	}
	if len(cards) == 0 {
		t.Error("board empty")
	}
	for _, c := range cards {
		for _, k := range []string{"ulid", "slug", "status", "rank", "card_word"} {
			if _, ok := c[k]; !ok {
				t.Errorf("card key %q missing", k)
			}
		}
	}
}

func TestHistoryEndpoint(t *testing.T) {
	s := svc(t)
	mustCreate(t, s, "hist-test")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/ticket/hist-test/history?limit=10", nil)
	NewHTTP(s).Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var evs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &evs); err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0]["kind"] != "ticket.create" {
		t.Errorf("history wrong: %v", evs)
	}
}

func TestSSEStreamDeliversChange(t *testing.T) {
	s := svc(t)
	id := mustCreate(t, s, "sse-test")
	h := NewHTTP(s).Routes()

	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// give the LISTEN a moment to attach, then write
	time.Sleep(200 * time.Millisecond)
	if _, err := s.Append(ctx, WriteCmd{Ticket: id, Kind: "status.set", Actor: "agent:test",
		Payload: map[string]any{"status": "active"}, Session: "sse"}); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, `"kind":"status.set"`) {
		t.Errorf("SSE payload missing change: %q", out)
	}
}

func mustCreate(t *testing.T, s Service, slug string) string {
	t.Helper()
	id := store.NewULID()
	if _, err := s.Append(context.Background(), WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": slug, "title": "T " + slug},
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
