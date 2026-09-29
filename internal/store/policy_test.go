package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// setTier classifies a ticket the way the CLI does: servitor set <ref> tier=N.
func setTier(t *testing.T, s *Store, ticket string, tier any) {
	t.Helper()
	mustAppend(t, s, evt(ticket, "field.set", map[string]any{"field": "tier", "v": tier}))
}

func humanEvt(ticket, kind string, payload map[string]any) Event {
	return Event{TicketULID: ticket, Actor: "human:preston", ActorType: "human", Kind: kind, Payload: payload}
}

func ledgerRows(t *testing.T, s *Store, ticket string) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger WHERE ticket_ulid=$1`, ticket).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTierRequired(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "tier-req")
	active := evt(id, "status.set", map[string]any{"status": "active"})

	// absent first, then every wrong shape in turn
	for _, c := range []struct {
		name string
		tier any
	}{{"absent", nil}, {"four", 4}, {"word", "two"}, {"negative", -1}, {"fraction", 1.5}, {"string four", "4"}} {
		if c.tier != nil {
			setTier(t, s, id, c.tier)
		}
		before := ledgerRows(t, s, id)
		_, err := s.AppendEvent(ctx, active, time.Time{})
		if !errors.Is(err, ErrTierRequired) {
			t.Fatalf("tier %s: want ErrTierRequired, got %v", c.name, err)
		}
		if ledgerRows(t, s, id) != before {
			t.Fatalf("tier %s: a rejected active wrote a ledger row", c.name)
		}
	}
	for _, tier := range []int{0, 1, 2, 3} {
		id := newTicket(t, s, "tier-ok-"+string(rune('0'+tier)))
		setTier(t, s, id, tier)
		mustAppend(t, s, evt(id, "status.set", map[string]any{"status": "active"}))
	}
	// the CLI's field=value sends a string; "2" is a classification too
	str := newTicket(t, s, "tier-str")
	setTier(t, s, str, "2")
	mustAppend(t, s, evt(str, "status.set", map[string]any{"status": "active"}))
	// tier in the create payload counts too, and a bad one is refused
	created := NewULID()
	mustAppend(t, s, evt(created, "ticket.create", map[string]any{"slug": "tier-create", "tier": 1}))
	mustAppend(t, s, evt(created, "status.set", map[string]any{"status": "active"}))
	_, err := s.AppendEvent(ctx, evt(NewULID(), "ticket.create", map[string]any{"slug": "tier-create-bad", "tier": 9}), time.Time{})
	expectFail(t, "ticket.create with tier 9", err, "invalid tier")
	// blocked, queued and dropped never ask for a tier
	un := newTicket(t, s, "tier-free")
	mustAppend(t, s, evt(un, "status.set", map[string]any{"status": "blocked", "on": "human"}))
	mustAppend(t, s, evt(un, "status.set", map[string]any{"status": "queued"}))
	mustAppend(t, s, evt(un, "status.set", map[string]any{"status": "dropped"}))
}

func TestContractRequired(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	head := func(id string) error {
		_, err := s.AppendEvent(ctx, evt(id, "field.set", map[string]any{"field": "head", "v": "abc1234"}), time.Time{})
		return err
	}
	// no tier: the tier rule speaks first
	none := newTicket(t, s, "cr-none")
	if err := head(none); !errors.Is(err, ErrTierRequired) {
		t.Fatalf("head without tier: want ErrTierRequired, got %v", err)
	}
	// tier 0 and 1 commit without a gate
	for _, tier := range []int{0, 1} {
		id := newTicket(t, s, "cr-low-"+string(rune('0'+tier)))
		setTier(t, s, id, tier)
		if err := head(id); err != nil {
			t.Fatalf("tier %d head: %v", tier, err)
		}
	}
	// tier 2 and 3 need the gate; after it, the commit lands
	for _, tier := range []int{2, 3} {
		id := newTicket(t, s, "cr-high-"+string(rune('0'+tier)))
		setTier(t, s, id, tier)
		before := ledgerRows(t, s, id)
		if err := head(id); !errors.Is(err, ErrContractRequired) {
			t.Fatalf("tier %d head before gate: want ErrContractRequired, got %v", tier, err)
		}
		if ledgerRows(t, s, id) != before {
			t.Fatal("a rejected head wrote a ledger row")
		}
		// clearing the field is bookkeeping, not a commit
		mustAppend(t, s, evt(id, "field.set", map[string]any{"field": "head"}))
		mustAppend(t, s, humanEvt(id, "gate", map[string]any{"gate": "contract_approved"}))
		if err := head(id); err != nil {
			t.Fatalf("tier %d head after gate: %v", tier, err)
		}
	}
}

func TestCriteriaIncomplete(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	done := func(id string) error {
		_, err := s.AppendEvent(ctx, evt(id, "status.set", map[string]any{"status": "done"}), time.Time{})
		return err
	}
	// zero criteria: done is free
	bare := newTicket(t, s, "ci-bare")
	if err := done(bare); err != nil {
		t.Fatalf("done with no criteria: %v", err)
	}
	// one unset, one failed, then all pass
	id := newTicket(t, s, "ci-crit")
	c1 := addSubitem(t, s, id, "criterion", "one")
	c2 := addSubitem(t, s, id, "criterion", "two")
	before := ledgerRows(t, s, id)
	if err := done(id); !errors.Is(err, ErrCriteriaIncomplete) {
		t.Fatalf("done with unset criteria: want ErrCriteriaIncomplete, got %v", err)
	}
	if ledgerRows(t, s, id) != before {
		t.Fatal("a rejected done wrote a ledger row")
	}
	mustAppend(t, s, evt(id, "subitem.set", map[string]any{"ulid": c1, "state": "pass"}))
	mustAppend(t, s, evt(id, "subitem.set", map[string]any{"ulid": c2, "state": "fail"}))
	if err := done(id); !errors.Is(err, ErrCriteriaIncomplete) {
		t.Fatalf("done with a failed criterion: want ErrCriteriaIncomplete, got %v", err)
	}
	mustAppend(t, s, evt(id, "subitem.set", map[string]any{"ulid": c2, "state": "pass"}))
	if err := done(id); err != nil {
		t.Fatalf("done with all pass: %v", err)
	}
	// plan steps and findings are not criteria
	other := newTicket(t, s, "ci-other")
	addSubitem(t, s, other, "plan", "step")
	addSubitem(t, s, other, "finding", "f")
	if err := done(other); err != nil {
		t.Fatalf("done with only non-criteria sub-items: %v", err)
	}
}

func TestDoneWaiver(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "waive")
	addSubitem(t, s, id, "criterion", "never proven")

	_, err := s.AppendEvent(ctx, evt(id, "status.set", map[string]any{"status": "done", "waive": "agent says so"}), time.Time{})
	if !errors.Is(err, ErrHumanWaiverRequired) {
		t.Fatalf("agent waive: want ErrHumanWaiverRequired, got %v", err)
	}
	_, err = s.AppendEvent(ctx, humanEvt(id, "status.set", map[string]any{"status": "done", "waive": ""}), time.Time{})
	if !errors.Is(err, ErrCriteriaIncomplete) {
		t.Fatalf("empty waive: want ErrCriteriaIncomplete, got %v", err)
	}
	mustAppend(t, s, humanEvt(id, "status.set", map[string]any{"status": "done", "waive": "accepted the local proof"}))

	var status string
	if err := s.Pool.QueryRow(ctx, `SELECT status::text FROM tickets WHERE ulid=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "done" {
		t.Fatalf("status %s, want done", status)
	}
	evs, err := s.History(ctx, id, 100)
	if err != nil {
		t.Fatal(err)
	}
	last := evs[len(evs)-1]
	if last.Kind != "status.set" || last.Actor != "human:preston" || last.Payload["waive"] != "accepted the local proof" {
		t.Fatalf("history does not carry the waive: %+v", last)
	}
}

// TestReplayPredatesRules: rows the rules would refuse today, written
// before the rules existed, still rebuild. Replay re-projects what was
// accepted; it never re-adjudicates.
func TestReplayPredatesRules(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "pre-rules")
	crit := addSubitem(t, s, id, "criterion", "unproven")
	mustAppend(t, s, evt(id, "subitem.set", map[string]any{"ulid": crit, "state": "fail"}))
	// raw ledger rows: only the projection tables are guarded
	for _, row := range []struct{ kind, payload string }{
		{"status.set", `{"status":"active"}`},
		{"field.set", `{"field":"head","v":"deadbee"}`},
		{"status.set", `{"status":"done"}`},
	} {
		if _, err := s.Pool.Exec(ctx,
			`INSERT INTO ledger (ulid, ticket_ulid, actor, actor_type, kind, payload) VALUES ($1,$2,'agent:old','agent',$3,$4)`,
			NewULID(), id, row.kind, row.payload); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := s.Replay(ctx, ReplayOptions{})
	if err != nil {
		t.Fatalf("strict replay over pre-rule rows: %v", err)
	}
	if len(rep.Skipped) != 0 {
		t.Fatalf("skipped %+v", rep.Skipped)
	}
	var status, head string
	if err := s.Pool.QueryRow(ctx, `SELECT status::text, fields->>'head' FROM tickets WHERE ulid=$1`, id).Scan(&status, &head); err != nil {
		t.Fatal(err)
	}
	if status != "done" || head != "deadbee" {
		t.Fatalf("rebuilt ticket: status %s head %s", status, head)
	}
	// and the live path still refuses the same thing today
	_, err = s.AppendEvent(ctx, evt(id, "status.set", map[string]any{"status": "active"}), time.Time{})
	if !errors.Is(err, ErrTierRequired) {
		t.Fatalf("live active without tier after replay: want ErrTierRequired, got %v", err)
	}
}
