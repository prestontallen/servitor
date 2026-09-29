package api

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The REST golden test pins every read route's JSON for one fixture, so a
// change that shares code with REST (store.LedgerFilter, the Service) can
// prove REST did not move. Regenerate with: go test ./internal/api -run
// TestRESTGolden -update — and only when a REST change is the intent.
var update = flag.Bool("update", false, "rewrite REST golden files")

// Fixed ticket ULIDs keep the fixture's identities stable across runs;
// server-minted ULIDs and timestamps are masked instead.
const (
	fxArc   = "01J0000000000000000000ARC1"
	fxAlpha = "01J00000000000000000A1FA01"
	fxBeta  = "01J000000000000000000BETA1"
	fxDone  = "01J000000000000000000D0NE1"
	fxCrit  = "01J00000000000000000CR1T01"
)

// goldenFixture writes a small ledger that touches every read shape: an arc
// with members, statuses across lanes, a human gate, a criterion with
// evidence, notes, a decision, a field and feedback.
func goldenFixture(t *testing.T, s Service) {
	t.Helper()
	ctx := context.Background()
	steps := []WriteCmd{
		{Ticket: fxArc, Kind: "ticket.create", Payload: map[string]any{"slug": "fx-arc", "title": "Fixture arc"}},
		{Ticket: fxAlpha, Kind: "ticket.create", Payload: map[string]any{"slug": "fx-alpha", "title": "Alpha", "tier": 1}},
		{Ticket: fxBeta, Kind: "ticket.create", Payload: map[string]any{"slug": "fx-beta", "title": "Beta"}},
		{Ticket: fxDone, Kind: "ticket.create", Payload: map[string]any{"slug": "fx-done", "title": "Done one"}},
		{Ticket: fxAlpha, Kind: "field.set", Payload: map[string]any{"field": "parent", "v": fxArc}},
		{Ticket: fxBeta, Kind: "field.set", Payload: map[string]any{"field": "parent", "v": fxArc}},
		{Ticket: fxAlpha, Kind: "status.set", Payload: map[string]any{"status": "active"}},
		{Ticket: fxAlpha, Kind: "gate", Actor: "human:preston", Payload: map[string]any{"gate": "contract_approved"}},
		{Ticket: fxAlpha, Kind: "subitem.add", Payload: map[string]any{"ulid": fxCrit, "kind": "criterion", "rank": 0, "body": "when x then y"}},
		{Ticket: fxAlpha, Kind: "subitem.set", Payload: map[string]any{"ulid": fxCrit, "state": "pass", "evidence": "a test"}},
		{Ticket: fxAlpha, Kind: "note", Payload: map[string]any{"v": "a note"}},
		{Ticket: fxAlpha, Kind: "decision", Payload: map[string]any{"what": "pick a", "why": "because"}},
		{Ticket: fxAlpha, Kind: "field.set", Payload: map[string]any{"field": "branch", "v": "agent/test/fx-alpha"}},
		{Ticket: fxBeta, Kind: "status.set", Payload: map[string]any{"status": "blocked", "on": "human"}},
		{Ticket: fxBeta, Kind: "feedback", Payload: map[string]any{"source": "human", "finding": "one line"}},
		{Ticket: fxDone, Kind: "status.set", Payload: map[string]any{"status": "done"}},
	}
	for _, c := range steps {
		if c.Actor == "" {
			c.Actor = "agent:test"
		}
		if _, err := s.Append(ctx, c); err != nil {
			t.Fatalf("fixture %s on %s: %v", c.Kind, c.Ticket, err)
		}
	}
}

var (
	tsRe   = regexp.MustCompile(`"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})"`)
	ulidRe = regexp.MustCompile(`"[0-9A-HJKMNP-TV-Z]{26}"`)
	fixed  = map[string]bool{`"` + fxArc + `"`: true, `"` + fxAlpha + `"`: true, `"` + fxBeta + `"`: true, `"` + fxDone + `"`: true, `"` + fxCrit + `"`: true}
)

// mask replaces run-dependent values (timestamps, server-minted ULIDs) and
// re-indents, leaving key order and every other byte as served.
func mask(b []byte) []byte {
	b = tsRe.ReplaceAll(b, []byte(`"<ts>"`))
	b = ulidRe.ReplaceAllFunc(b, func(m []byte) []byte {
		if fixed[string(m)] {
			return m
		}
		return []byte(`"<ulid>"`)
	})
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return b
	}
	out.WriteByte('\n')
	return out.Bytes()
}

func TestRESTGolden(t *testing.T) {
	s := svc(t)
	goldenFixture(t, s)
	h := NewHTTP(s).Routes()
	routes := []string{
		"/api/board",
		"/api/tickets",
		"/api/tickets?status=done",
		"/api/arcs",
		"/api/events",
		"/api/events?kind=note",
		"/api/events?ticket=fx-alpha&limit=3",
		"/api/events?before_id=8&limit=2",
		"/api/events?actor_type=human",
		"/api/ticket/fx-alpha",
		"/api/ticket/fx-alpha/history",
		"/api/ticket/fx-arc",
		"/api/feedback",
	}
	for _, route := range routes {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", route, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: status %d: %s", route, rec.Code, rec.Body.String())
		}
		got := mask(rec.Body.Bytes())
		name := strings.NewReplacer("/api/", "", "/", "_", "?", "__", "&", "_", "=", "-").Replace(route) + ".json"
		path := filepath.Join("testdata", "rest_golden", name)
		if *update {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run with -update to create)", route, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: JSON differs from %s\n got: %s", route, path, got)
		}
	}
}
