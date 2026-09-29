package store

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// mixedHistory writes the shapes replay has to reproduce: a slug rename,
// every gate, sub-items added without a ulid and then addressed by prefix
// (a body edit, evidence, a rank), a note addressed by prefix, a deduped
// decision, blocked/active, absent-vs-empty pr, free fields, an arc member,
// and ledger-only kinds. Returns the two ticket ulids.
func mixedHistory(t *testing.T, s *Store) (string, string) {
	t.Helper()
	ctx := context.Background()
	a := NewULID()
	mustAppend(t, s, evt(a, "ticket.create", map[string]any{"slug": "rp-a", "title": "A", "rank": 3}))
	mustAppend(t, s, evt(a, "note", map[string]any{"v": "Intake: tier 1"}))
	mustAppend(t, s, evt(a, "subitem.add", map[string]any{"kind": "criterion", "body": "when x then y"}))
	mustAppend(t, s, evt(a, "subitem.add", map[string]any{"kind": "criterion", "body": "when p then q", "rank": 2}))
	mustAppend(t, s, evt(a, "subitem.add", map[string]any{"kind": "plan", "body": "step 1"}))
	mustAppend(t, s, evt(a, "subitem.add", map[string]any{"kind": "question", "body": "q?", "ulid": NewULID()}))
	var crit1, crit2, plan, note1 string
	for _, q := range []struct {
		kind, body string
		dst        *string
	}{{"criterion", "when x then y", &crit1}, {"criterion", "when p then q", &crit2}, {"plan", "step 1", &plan}, {"note", "Intake: tier 1", &note1}} {
		if err := s.Pool.QueryRow(ctx, `SELECT ulid FROM subitems WHERE ticket_ulid=$1 AND kind=$2 AND body=$3`, a, q.kind, q.body).Scan(q.dst); err != nil {
			t.Fatal(err)
		}
	}
	mustAppend(t, s, evt(a, "subitem.set", map[string]any{"ulid": crit1[:12], "body": "when x then y (edited)"}))
	mustAppend(t, s, evt(a, "subitem.set", map[string]any{"ulid": crit1[:10], "state": "pass", "evidence": "go test"}))
	mustAppend(t, s, evt(a, "subitem.set", map[string]any{"ulid": crit2, "state": "fail"}))
	mustAppend(t, s, evt(a, "subitem.rank", map[string]any{"ulid": plan[:14], "rank": 9}))
	mustAppend(t, s, evt(a, "subitem.set", map[string]any{"ulid": note1[:12], "body": "Intake: tier 1 (amended)"}))
	mustAppend(t, s, evt(a, "decision", map[string]any{"what": "lock", "why": "db enforces"}))
	mustAppend(t, s, evt(a, "decision", map[string]any{"what": "lock", "why": "db enforces"})) // deduped
	mustAppend(t, s, evt(a, "decision", map[string]any{"what": "key", "why": "unique"}))
	mustAppend(t, s, evt(a, "contract", map[string]any{"intent": "rebuild", "in": []string{"x"}}))
	mustAppend(t, s, Event{TicketULID: a, Actor: "human:preston", ActorType: "human", Kind: "gate", Payload: map[string]any{"gate": "contract_approved"}})
	mustAppend(t, s, evt(a, "field.set", map[string]any{"field": "next", "v": "build"}))
	mustAppend(t, s, evt(a, "field.set", map[string]any{"field": "pr", "v": ""}))
	mustAppend(t, s, evt(a, "field.set", map[string]any{"field": "pr"}))
	mustAppend(t, s, evt(a, "field.set", map[string]any{"field": "slug", "v": "rp-a2"}))
	mustAppend(t, s, evt(a, "status.set", map[string]any{"status": "blocked", "on": "human"}))
	mustAppend(t, s, evt(a, "status.set", map[string]any{"status": "active"}))
	mustAppend(t, s, evt(a, "review", map[string]any{"verdict": "present", "summary": "2/2"}))
	mustAppend(t, s, evt(a, "feedback", map[string]any{"finding": "f", "source": "self"}))
	mustAppend(t, s, evt(a, "somecustomkind", map[string]any{"x": 1}))
	mustAppend(t, s, evt(a, "gate", map[string]any{"gate": "presented"}))

	b := NewULID()
	mustAppend(t, s, evt(b, "ticket.create", map[string]any{"slug": "rp-b", "title": "B"}))
	mustAppend(t, s, evt(b, "field.set", map[string]any{"field": "parent", "v": a}))
	mustAppend(t, s, evt(b, "note", map[string]any{"v": "member"}))
	mustAppend(t, s, evt(b, "status.set", map[string]any{"status": "done"}))
	mustAppend(t, s, evt(b, "gate", map[string]any{"gate": "shipped"}))
	return a, b
}

// readModel is every read surface the projection feeds, as one byte string.
func readModel(t *testing.T, s *Store, tickets ...string) []byte {
	t.Helper()
	ctx := context.Background()
	var out bytes.Buffer
	for _, id := range tickets {
		doc, err := s.CtxRead(ctx, id)
		if err != nil {
			t.Fatalf("ctx %s: %v", id, err)
		}
		out.Write(doc)
		out.WriteByte('\n')
	}
	for _, r := range []struct {
		name string
		read func() (any, error)
	}{
		{"board", func() (any, error) { return s.Board(ctx) }},
		{"arcs", func() (any, error) { return s.Arcs(ctx) }},
		{"list", func() (any, error) { return s.List(ctx, ListFilter{}) }},
	} {
		v, err := r.read()
		if err != nil {
			t.Fatalf("%s: %v", r.name, err)
		}
		j, _ := json.Marshal(v)
		out.WriteString(r.name + " ")
		out.Write(j)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func tableCounts(t *testing.T, s *Store) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	counts := map[string]int64{}
	for _, tbl := range []string{"tickets", "slug_history", "subitems", "gate_events", "ledger"} {
		var n int64
		if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM `+tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[tbl] = n
	}
	return counts
}

func TestReplayRoundTrip(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a, b := mixedHistory(t, s)
	before := readModel(t, s, a, b)
	countsBefore := tableCounts(t, s)

	rep, err := s.Replay(ctx, ReplayOptions{})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	after := readModel(t, s, a, b)
	if !bytes.Equal(before, after) {
		t.Fatalf("read model differs after replay\nbefore: %s\nafter:  %s", before, after)
	}
	if countsAfter := tableCounts(t, s); countsAfter["subitems"] != countsBefore["subitems"] ||
		countsAfter["slug_history"] != countsBefore["slug_history"] || countsAfter["gate_events"] != countsBefore["gate_events"] {
		t.Fatalf("table counts differ: before %v after %v", countsBefore, countsAfter)
	}
	// minted identities: 3 ulid-less adds + 2 notes + 2 distinct decisions
	if rep.Recovered != 7 || rep.Fresh != 0 || len(rep.Skipped) != 0 || rep.Rows != countsBefore["ledger"] {
		t.Fatalf("report %+v, want 7 recovered, 0 fresh, 0 skipped, %d rows", rep, countsBefore["ledger"])
	}
	var maxID, mark int64
	if err := s.Pool.QueryRow(ctx, `SELECT max(id), (SELECT last_id FROM ledger_watermark) FROM ledger`).Scan(&maxID, &mark); err != nil {
		t.Fatal(err)
	}
	if mark != maxID || rep.Watermark != maxID {
		t.Fatalf("watermark %d, report %d, want max id %d", mark, rep.Watermark, maxID)
	}
	// both slugs still claim the ticket, the live one resolves
	if id, err := s.TicketBySlug(ctx, "RP-A2"); err != nil || id != a {
		t.Fatalf("slug rp-a2 -> %q, %v", id, err)
	}
	var claims int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM slug_history WHERE ticket_ulid=$1`, a).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if claims != 2 {
		t.Fatalf("slug_history claims for a: %d, want 2", claims)
	}
	// the replay changed nothing about the ledger itself
	if tableCounts(t, s)["ledger"] != countsBefore["ledger"] {
		t.Fatal("ledger row count changed")
	}
}

// TestReplaySubitemIdentity is the design killer named in the contract: a
// sub-item added without a ulid, its body edited, then addressed by prefix.
// After replay the same ulid is there and the prefix writes still show.
func TestReplaySubitemIdentity(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a, _ := mixedHistory(t, s)
	// values, not pointers: the comparison below is by ==
	type sub struct {
		ULID, Body, State, Evidence string
	}
	read := func() (crit1 sub, note string) {
		var doc struct {
			Criteria []struct {
				ULID, Body      string
				State, Evidence *string
			} `json:"criteria"`
			Notes []struct {
				Body string `json:"body"`
			} `json:"notes"`
		}
		raw, err := s.CtxRead(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		c := doc.Criteria[0]
		deref := func(p *string) string {
			if p == nil {
				return "<nil>"
			}
			return *p
		}
		return sub{c.ULID, c.Body, deref(c.State), deref(c.Evidence)}, doc.Notes[len(doc.Notes)-1].Body
	}
	crit, note := read()
	if crit.Body != "when x then y (edited)" || crit.State != "pass" || crit.Evidence != "go test" || note != "Intake: tier 1 (amended)" {
		t.Fatalf("history did not land as expected: %+v %q", crit, note)
	}
	if _, err := s.Replay(ctx, ReplayOptions{}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	crit2, note2 := read()
	if crit2 != crit || note2 != note {
		t.Fatalf("identity lost: before %+v %q, after %+v %q", crit, note, crit2, note2)
	}
	// and the ulid still answers to its prefix on the live write path
	mustAppend(t, s, evt(a, "subitem.set", map[string]any{"ulid": crit.ULID[:10], "state": "fail"}))
}

// TestReplaySlugHistory: a renamed ticket replays both claims in order and
// the live slug is the later one.
func TestReplaySlugHistory(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a, _ := mixedHistory(t, s)
	if _, err := s.Replay(ctx, ReplayOptions{}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	rows, err := s.Pool.Query(ctx, `SELECT slug_lower FROM slug_history WHERE ticket_ulid=$1 ORDER BY claimed_at`, a)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var sl string
		if err := rows.Scan(&sl); err != nil {
			t.Fatal(err)
		}
		slugs = append(slugs, sl)
	}
	if strings.Join(slugs, ",") != "rp-a,rp-a2" {
		t.Fatalf("slug_history for a: %v, want rp-a then rp-a2", slugs)
	}
	for _, sl := range []string{"rp-a", "rp-a2", "RP-A2"} {
		if id, err := s.TicketBySlug(ctx, sl); err != nil || id != a {
			t.Fatalf("%s -> %q, %v", sl, id, err)
		}
	}
	var live string
	if err := s.Pool.QueryRow(ctx, `SELECT slug FROM tickets WHERE ulid=$1`, a).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != "rp-a2" {
		t.Fatalf("live slug %q, want rp-a2", live)
	}
	// the slug stays claimed history-wide after replay
	_, err = s.AppendEvent(ctx, evt(NewULID(), "ticket.create", map[string]any{"slug": "rp-a"}), time.Time{})
	expectFail(t, "reuse of the pre-rename slug after replay", err, "claimed")
}

func TestReplayDryRunChangesNothing(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a, b := mixedHistory(t, s)
	before := readModel(t, s, a, b)
	var markBefore int64
	if err := s.Pool.QueryRow(ctx, `SELECT last_id FROM ledger_watermark`).Scan(&markBefore); err != nil {
		t.Fatal(err)
	}

	rep, err := s.Replay(ctx, ReplayOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !rep.DryRun || rep.Rows != tableCounts(t, s)["ledger"] || rep.Recovered != 7 || len(rep.Skipped) != 0 {
		t.Fatalf("dry-run report %+v", rep)
	}
	if after := readModel(t, s, a, b); !bytes.Equal(before, after) {
		t.Fatal("dry run changed the read model")
	}
	var markAfter int64
	if err := s.Pool.QueryRow(ctx, `SELECT last_id FROM ledger_watermark`).Scan(&markAfter); err != nil {
		t.Fatal(err)
	}
	if markAfter != markBefore {
		t.Fatalf("dry run moved the watermark %d -> %d", markBefore, markAfter)
	}
	// and a real run afterwards still works (the dry run left no lock or state behind)
	if _, err := s.Replay(ctx, ReplayOptions{}); err != nil {
		t.Fatalf("replay after dry run: %v", err)
	}
}

func TestReplayStrictRefusesLossySkips(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a, b := mixedHistory(t, s)
	// a row the projection never accepted: the ledger takes it raw (only
	// the projection tables are guarded), so replay meets a prefix that
	// resolves to nothing.
	var badID int64
	if err := s.Pool.QueryRow(ctx,
		`INSERT INTO ledger (ulid, ticket_ulid, actor, actor_type, kind, payload)
		 VALUES ($1,$2,'agent:test','agent','subitem.set','{"ulid":"ZZZZZZ","state":"pass"}') RETURNING id`,
		NewULID(), a).Scan(&badID); err != nil {
		t.Fatal(err)
	}
	// snapshot after the insert: ctx.head is max(ledger.id), which the raw
	// row moved, and that is not the replay's doing
	before := readModel(t, s, a, b)

	_, err := s.Replay(ctx, ReplayOptions{})
	expectFail(t, "strict replay on a row that cannot re-apply", err, "no subitem matches")
	if err != nil && !strings.Contains(err.Error(), "event "+strconv.FormatInt(badID, 10)) {
		t.Fatalf("error does not name the event: %v", err)
	}
	if after := readModel(t, s, a, b); !bytes.Equal(before, after) {
		t.Fatal("a refused replay changed the read model")
	}

	rep, err := s.Replay(ctx, ReplayOptions{Lossy: true})
	if err != nil {
		t.Fatalf("lossy replay: %v", err)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0].ID != badID || rep.Skipped[0].Kind != "subitem.set" {
		t.Fatalf("skipped %+v, want the one bad row %d", rep.Skipped, badID)
	}
	// the bad row changed nothing when it was skipped, so the model is the
	// same except updated_at: applyRow's stamp is inside the savepoint too.
	if after := readModel(t, s, a, b); !bytes.Equal(before, after) {
		t.Fatalf("lossy replay differs from the model before the bad row\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestReplayFromLedgerAlone(t *testing.T) {
	// The disaster case: the read model is gone and only the ledger
	// remains. Identities cannot be recovered; strict names the first
	// prefix that no longer resolves, lossy rebuilds everything that does
	// not depend on a minted ulid and reports the rest.
	s := testDB(t)
	ctx := context.Background()
	a, b := mixedHistory(t, s)
	if _, err := s.Pool.Exec(ctx, `TRUNCATE tickets, slug_history, subitems, gate_events`); err != nil {
		t.Fatal(err)
	}
	_, err := s.Replay(ctx, ReplayOptions{})
	expectFail(t, "strict replay with no read model to recover from", err, "no subitem matches")

	rep, err := s.Replay(ctx, ReplayOptions{Lossy: true})
	if err != nil {
		t.Fatalf("lossy: %v", err)
	}
	// 5 references to minted sub-items are lost: 2 on crit1, the full-ulid
	// set on crit2 (its add carried no ulid either), the rank on plan, the
	// note edit. Only the question, whose add carried a ulid, keeps its
	// identity, and nothing references it.
	if rep.Fresh != 7 || rep.Recovered != 0 || len(rep.Skipped) != 5 {
		t.Fatalf("report %+v, want 7 fresh, 0 recovered, 5 skipped", rep)
	}
	doc, err := s.CtxRead(ctx, "rp-a2")
	if err != nil {
		t.Fatalf("ticket a after lossy rebuild: %v", err)
	}
	var got struct {
		Status   string `json:"status"`
		CardWord string `json:"card_word"`
		Criteria []struct {
			State *string `json:"state"`
		} `json:"criteria"`
		Gates     []any `json:"gates"`
		Decisions []any `json:"decisions"`
	}
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "active" || got.CardWord != "checking" || len(got.Criteria) != 2 || len(got.Gates) != 2 || len(got.Decisions) != 2 {
		t.Fatalf("rebuilt ticket a: %s", doc)
	}
	arcs, err := s.Arcs(ctx)
	if err != nil || len(arcs) != 1 || arcs[0].ULID != a || len(arcs[0].Members) != 1 || arcs[0].Members[0].ULID != b {
		t.Fatalf("arcs after rebuild: %+v, %v", arcs, err)
	}
}

func TestReplayBlocksConcurrentAppend(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	a, _ := mixedHistory(t, s)

	reached := make(chan struct{})
	release := make(chan struct{})
	s.afterReplayLock = func() {
		close(reached)
		<-release
	}
	replayDone := make(chan error, 1)
	go func() {
		_, err := s.Replay(ctx, ReplayOptions{})
		replayDone <- err
	}()
	<-reached

	type result struct {
		id  int64
		err error
	}
	appended := make(chan result, 1)
	go func() {
		id, err := s.AppendEvent(ctx, evt(a, "note", map[string]any{"v": "during replay"}), time.Time{})
		appended <- result{id, err}
	}()
	select {
	case r := <-appended:
		t.Fatalf("append landed during replay: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-replayDone; err != nil {
		t.Fatalf("replay: %v", err)
	}
	r := <-appended
	if r.err != nil {
		t.Fatalf("append after replay: %v", r.err)
	}
	var mark int64
	if err := s.Pool.QueryRow(ctx, `SELECT last_id FROM ledger_watermark`).Scan(&mark); err != nil {
		t.Fatal(err)
	}
	if mark != r.id {
		t.Fatalf("watermark %d, want the append's id %d", mark, r.id)
	}
	doc, err := s.CtxRead(ctx, a)
	if err != nil || !bytes.Contains(doc, []byte("during replay")) {
		t.Fatalf("the append is not in the rebuilt model: %v %s", err, doc)
	}
}
