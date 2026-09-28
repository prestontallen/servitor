package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
	"github.com/prestontallen/servitor/internal/testdb"
)

// countingService wraps a Service and counts reads, so a test can prove
// how many ledger reads a query cost, or that it cost none.
type countingService struct {
	api.Service
	events atomic.Int64
	reads  atomic.Int64
}

func (c *countingService) Events(ctx context.Context, f store.LedgerFilter) ([]store.LedgerEvent, error) {
	c.events.Add(1)
	c.reads.Add(1)
	return c.Service.Events(ctx, f)
}

func (c *countingService) Board(ctx context.Context) ([]store.Card, error) {
	c.reads.Add(1)
	return c.Service.Board(ctx)
}

func (c *countingService) List(ctx context.Context, f store.ListFilter) ([]store.Card, error) {
	c.reads.Add(1)
	return c.Service.List(ctx, f)
}

func (c *countingService) Ctx(ctx context.Context, ref string) (json.RawMessage, error) {
	c.reads.Add(1)
	return c.Service.Ctx(ctx, ref)
}

func (c *countingService) Arcs(ctx context.Context) ([]store.ArcSummary, error) {
	c.reads.Add(1)
	return c.Service.Arcs(ctx)
}

func testService(t *testing.T) *countingService {
	t.Helper()
	ctx := context.Background()
	adminDSN := testdb.AdminDSN(t)
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	defer admin.Close(ctx)
	for _, q := range []string{
		`DROP DATABASE IF EXISTS servitor_graphql_test WITH (FORCE)`,
		`CREATE DATABASE servitor_graphql_test`,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	dsn := os.Getenv("SERVITOR_TEST_DSN")
	if dsn == "" {
		dsn = testdb.Named(t, adminDSN, "servitor_graphql_test")
	}
	s, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Pool.Close() })
	return &countingService{Service: api.NewStoreService(s)}
}

func appendOK(t *testing.T, s api.Service, ticket, kind string, payload map[string]any) {
	t.Helper()
	if _, err := s.Append(context.Background(), api.WriteCmd{Ticket: ticket, Kind: kind, Actor: "agent:test", Payload: payload}); err != nil {
		t.Fatalf("%s on %s: %v", kind, ticket, err)
	}
}

func create(t *testing.T, s api.Service, slug string) string {
	t.Helper()
	id := store.NewULID()
	appendOK(t, s, id, "ticket.create", map[string]any{"slug": slug, "title": "T " + slug})
	return id
}

func noteOn(t *testing.T, s api.Service, ticket, text string) {
	t.Helper()
	appendOK(t, s, ticket, "note", map[string]any{"v": text})
}

// between returns a time strictly between the ts of the events appended
// before and after the call.
func between() time.Time {
	time.Sleep(20 * time.Millisecond)
	at := time.Now()
	time.Sleep(20 * time.Millisecond)
	return at
}

type gqlResult struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func (r gqlResult) code() string {
	if len(r.Errors) == 0 {
		return ""
	}
	c, _ := r.Errors[0].Extensions["code"].(string)
	return c
}

func query(t *testing.T, svc api.Service, q string, vars map[string]any) gqlResult {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"query": q, "variables": vars})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/graphql", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:5555"
	h := api.NewHTTP(svc)
	h.GraphQL = NewHandler(svc)
	h.Routes().ServeHTTP(rec, req)
	var out gqlResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("status %d, body %s: %v", rec.Code, rec.Body.String(), err)
	}
	return out
}

func mustData(t *testing.T, r gqlResult, into any) {
	t.Helper()
	if len(r.Errors) > 0 {
		t.Fatalf("errors: %+v", r.Errors)
	}
	if err := json.Unmarshal(r.Data, into); err != nil {
		t.Fatal(err)
	}
}

type evPage struct {
	Events struct {
		Events []struct {
			ID   int    `json:"id"`
			Kind string `json:"kind"`
		} `json:"events"`
		Next *int `json:"next_before_id"`
	} `json:"events"`
}

func (p evPage) ids() []int {
	var out []int
	for _, e := range p.Events.Events {
		out = append(out, e.ID)
	}
	return out
}

func TestGraphQLEventsWindow(t *testing.T) {
	svc := testService(t)
	a := create(t, svc, "gw-a")
	noteOn(t, svc, a, "before")
	since := between()
	noteOn(t, svc, a, "in 1")
	noteOn(t, svc, a, "in 2")
	noteOn(t, svc, a, "in 3")
	until := between()
	noteOn(t, svc, a, "after")

	q := `query($s: Time, $u: Time, $n: Int) { events(since: $s, until: $u, limit: $n) { events { id kind } next_before_id } }`
	var full evPage
	mustData(t, query(t, svc, q, map[string]any{"s": since, "u": until, "n": 10}), &full)
	if len(full.Events.Events) != 3 || full.Events.Next != nil {
		t.Fatalf("window: got %v next %v, want 3 notes and no next page", full.ids(), full.Events.Next)
	}
	for i, e := range full.Events.Events {
		if e.Kind != "note" || (i > 0 && e.ID >= full.Events.Events[i-1].ID) {
			t.Fatalf("window not newest-first notes: %+v", full.Events.Events)
		}
	}

	// a full page carries the cursor; following it reaches the rest
	var p1, p2 evPage
	mustData(t, query(t, svc, q, map[string]any{"s": since, "u": until, "n": 2}), &p1)
	if p1.Events.Next == nil || *p1.Events.Next != p1.ids()[1] {
		t.Fatalf("page 1: %v next %v", p1.ids(), p1.Events.Next)
	}
	q2 := `query($s: Time, $u: Time, $b: Int) { events(since: $s, until: $u, before_id: $b, limit: 2) { events { id kind } next_before_id } }`
	mustData(t, query(t, svc, q2, map[string]any{"s": since, "u": until, "b": *p1.Events.Next}), &p2)
	if len(p2.Events.Events) != 1 || p2.Events.Next != nil || p2.ids()[0] != full.ids()[2] {
		t.Fatalf("page 2: %v next %v, want [%d] and no next", p2.ids(), p2.Events.Next, full.ids()[2])
	}
}

func TestGraphQLBatchesTicketEvents(t *testing.T) {
	svc := testService(t)
	const n = 12
	for i := 0; i < n; i++ {
		id := create(t, svc, fmt.Sprintf("gb-%02d", i))
		for j := 0; j < 3; j++ {
			noteOn(t, svc, id, fmt.Sprintf("t%d n%d", i, j))
		}
	}
	since := time.Now().Add(-time.Hour)

	svc.events.Store(0)
	var got struct {
		Board []struct {
			Slug   string `json:"slug"`
			Events []struct {
				Ticket string `json:"ticket_ulid"`
				Kind   string `json:"kind"`
			} `json:"events"`
			ULID string `json:"ulid"`
		} `json:"board"`
	}
	mustData(t, query(t, svc, `query($s: Time) { board { ulid slug events(since: $s, kind: "note", limit: 2) { ticket_ulid kind } } }`,
		map[string]any{"s": since}), &got)
	if len(got.Board) != n {
		t.Fatalf("board has %d tickets, want %d", len(got.Board), n)
	}
	for _, c := range got.Board {
		if len(c.Events) != 2 {
			t.Errorf("%s: %d events, want its newest 2", c.Slug, len(c.Events))
		}
		for _, e := range c.Events {
			if e.Ticket != c.ULID {
				t.Errorf("%s got an event of %s", c.Slug, e.Ticket)
			}
		}
	}
	if calls := svc.events.Load(); calls != 1 {
		t.Errorf("board with nested events made %d Events calls, want 1", calls)
	}

	// two argument sets are two reads, not 2n
	svc.events.Store(0)
	var two struct{}
	mustData(t, query(t, svc, `{ board { a: events(limit: 1) { id } b: events(limit: 2) { id } } }`, nil), &two)
	if calls := svc.events.Load(); calls != 2 {
		t.Errorf("two nested argument sets made %d Events calls, want 2", calls)
	}
}

func TestGraphQLTicketAndArcScope(t *testing.T) {
	svc := testService(t)
	arc := create(t, svc, "gs-arc")
	a := create(t, svc, "gs-a")
	b := create(t, svc, "gs-b")
	other := create(t, svc, "gs-other")
	appendOK(t, svc, a, "field.set", map[string]any{"field": "parent", "v": arc})
	appendOK(t, svc, b, "field.set", map[string]any{"field": "parent", "v": arc})
	for _, id := range []string{arc, a, b, other} {
		noteOn(t, svc, id, "hello")
	}

	ticketsOf := func(r gqlResult) map[string]int {
		var p struct {
			Events struct {
				Events []struct {
					Ticket string `json:"ticket_ulid"`
				} `json:"events"`
			} `json:"events"`
		}
		mustData(t, r, &p)
		out := map[string]int{}
		for _, e := range p.Events.Events {
			out[e.Ticket]++
		}
		return out
	}

	// a slug and a ULID mix
	got := ticketsOf(query(t, svc, `query($t: [String!]) { events(tickets: $t, kind: "note") { events { ticket_ulid } } }`,
		map[string]any{"t": []string{"gs-a", b}}))
	if len(got) != 2 || got[a] != 1 || got[b] != 1 {
		t.Errorf("tickets [gs-a, b]: %v", got)
	}

	// arc = the arc ticket and its members
	got = ticketsOf(query(t, svc, `{ events(arc: "gs-arc", kind: "note") { events { ticket_ulid } } }`, nil))
	if len(got) != 3 || got[other] != 0 {
		t.Errorf("arc gs-arc: %v", got)
	}

	// both: the intersection; disjoint is empty, not everything
	got = ticketsOf(query(t, svc, `{ events(arc: "gs-arc", tickets: ["gs-a", "gs-other"], kind: "note") { events { ticket_ulid } } }`, nil))
	if len(got) != 1 || got[a] != 1 {
		t.Errorf("arc AND tickets: %v", got)
	}
	got = ticketsOf(query(t, svc, `{ events(arc: "gs-arc", tickets: ["gs-other"]) { events { ticket_ulid } } }`, nil))
	if len(got) != 0 {
		t.Errorf("disjoint arc AND tickets: %v", got)
	}

	// unknown refs are unknown_ticket, as a slug and as a well-formed ULID
	for _, q := range []string{
		`{ events(tickets: ["gs-nope"]) { events { id } } }`,
		`{ events(tickets: ["` + store.NewULID() + `"]) { events { id } } }`,
		`{ events(arc: "gs-nope") { events { id } } }`,
		`{ ticket(ref: "gs-nope") { slug } }`,
	} {
		if r := query(t, svc, q, nil); r.code() != "unknown_ticket" {
			t.Errorf("%s: code %q, errors %+v", q, r.code(), r.Errors)
		}
	}

	// links: parent_ticket and children through the index
	var tk struct {
		Ticket struct {
			Children []struct{ Slug string } `json:"children"`
		} `json:"ticket"`
	}
	mustData(t, query(t, svc, `{ ticket(ref: "gs-arc") { children { slug } } }`, nil), &tk)
	if len(tk.Ticket.Children) != 2 {
		t.Errorf("gs-arc children: %+v", tk.Ticket.Children)
	}
	var pt struct {
		Ticket struct {
			Parent struct{ Slug string } `json:"parent_ticket"`
		} `json:"ticket"`
	}
	mustData(t, query(t, svc, `{ ticket(ref: "gs-a") { parent_ticket { slug } } }`, nil), &pt)
	if pt.Ticket.Parent.Slug != "gs-arc" {
		t.Errorf("gs-a parent_ticket: %+v", pt.Ticket.Parent)
	}
}

func TestGraphQLAggregateIsCtx(t *testing.T) {
	svc := testService(t)
	id := create(t, svc, "ga-one")
	noteOn(t, svc, id, "x")
	want, err := svc.Ctx(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Ticket struct {
			Aggregate json.RawMessage `json:"aggregate"`
		} `json:"ticket"`
	}
	mustData(t, query(t, svc, `{ ticket(ref: "ga-one") { aggregate } }`, nil), &got)
	// the response is compacted on the way out; key order and values are not touched
	var compact bytes.Buffer
	if err := json.Compact(&compact, want); err != nil {
		t.Fatal(err)
	}
	want = compact.Bytes()
	if string(got.Ticket.Aggregate) != string(want) {
		t.Errorf("aggregate differs from ctx:\n got %s\nwant %s", got.Ticket.Aggregate, want)
	}
}

func TestGraphQLCapsRejectBeforeReading(t *testing.T) {
	svc := testService(t)
	create(t, svc, "gc-a")
	create(t, svc, "gc-b")

	deep := `{ board { parent_ticket { parent_ticket { parent_ticket { parent_ticket { parent_ticket { parent_ticket { parent_ticket { parent_ticket { slug } } } } } } } } } }`
	var sb strings.Builder
	sb.WriteString("{ ")
	for i := 0; i < 130; i++ {
		fmt.Fprintf(&sb, "b%d: board { slug title status rank } ", i)
	}
	sb.WriteString("}")

	cases := []struct {
		name, q, code string
	}{
		{"depth", deep, "query_too_deep"},
		{"complexity", sb.String(), "COMPLEXITY_LIMIT_EXCEEDED"},
		{"limit over cap", `{ events(limit: 10001) { events { id } } }`, "invalid_event"},
		{"limit zero", `{ events(limit: 0) { events { id } } }`, "invalid_event"},
		{"nested tickets x limit over cap", `{ board { events(limit: 6000) { id } } }`, "invalid_event"},
	}
	for _, c := range cases {
		svc.events.Store(0)
		svc.reads.Store(0)
		r := query(t, svc, c.q, nil)
		if r.code() != c.code {
			t.Errorf("%s: code %q, want %q; errors %+v", c.name, r.code(), c.code, r.Errors)
		}
		if n := svc.events.Load(); n != 0 {
			t.Errorf("%s: %d ledger reads before rejecting, want 0", c.name, n)
		}
		if c.name != "nested tickets x limit over cap" {
			if n := svc.reads.Load(); n != 0 {
				t.Errorf("%s: %d store reads before rejecting, want 0", c.name, n)
			}
		}
	}
}

// The endpoint sits under the /api/ auth guard: a non-local caller without
// the token is refused before any resolver runs. No database needed.
func TestGraphQLRequiresTokenOffHost(t *testing.T) {
	h := api.NewHTTP(nil)
	h.Token = "secret"
	h.GraphQL = NewHandler(nil)
	body := `{"query":"{ board { slug } }"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/graphql", strings.NewReader(body))
	req.RemoteAddr = "192.0.2.10:4000"
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("off-host without token: status %d, want 401", rec.Code)
	}

	// GET is not served: POST only
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/graphql?query={board{slug}}", nil)
	req.RemoteAddr = "127.0.0.1:4000"
	h.Routes().ServeHTTP(rec, req)
	if rec.Code == 200 {
		t.Errorf("GET /api/graphql served 200; want it refused")
	}
}

// Every type and field the schema declares carries a description: the
// schema is the endpoint's documentation. Reads the schema compiled into
// the generated code, so it checks what is served.
func TestSchemaIsDescribed(t *testing.T) {
	schema := NewExecutableSchema(Config{Resolvers: &Resolver{}}).Schema()
	checked := 0
	for name, def := range schema.Types {
		if def.BuiltIn || strings.HasPrefix(name, "__") {
			continue
		}
		if strings.TrimSpace(def.Description) == "" {
			t.Errorf("type %s has no description", name)
		}
		for _, f := range def.Fields {
			if strings.HasPrefix(f.Name, "__") {
				continue
			}
			checked++
			if strings.TrimSpace(f.Description) == "" {
				t.Errorf("%s.%s has no description", name, f.Name)
			}
		}
	}
	if checked < 40 {
		t.Errorf("checked only %d fields; the schema did not load", checked)
	}
}
