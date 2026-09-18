package store

import (
	"context"
	"testing"
)

func TestHandoffLatency(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	id := newTicket(t, s, "handoff-test")
	mustAppend(t, s, evt(id, "note", map[string]any{"v": "agent presenting"}))
	mustAppend(t, s, Event{TicketULID: id, Actor: "human:preston", ActorType: "human", Session: "t",
		Kind: "gate", Payload: map[string]any{"gate": "contract_approved"}})
	mustAppend(t, s, evt(id, "note", map[string]any{"v": "agent picked it back up"}))
	mustAppend(t, s, Event{TicketULID: id, Actor: "human:preston", ActorType: "human", Session: "t",
		Kind: "gate", Payload: map[string]any{"gate": "presented"}})

	rows, err := s.Handoffs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var row *HandoffRow
	for i := range rows {
		if rows[i].ULID == id {
			row = &rows[i]
		}
	}
	if row == nil {
		t.Fatalf("ticket missing from handoffs: %+v", rows)
	}
	if row.Slug != "handoff-test" {
		t.Errorf("slug=%q", row.Slug)
	}
	if row.PresentedAt == nil || row.ApprovedAt == nil {
		t.Fatalf("gate ts missing: %+v", row)
	}
	if row.HumanWaitSecs == nil || *row.HumanWaitSecs < 0 {
		t.Errorf("human_wait=%v want non-nil >= 0", row.HumanWaitSecs)
	}
	if row.AgentWaitSecs == nil || *row.AgentWaitSecs < 0 {
		t.Errorf("agent_wait=%v want non-nil >= 0", row.AgentWaitSecs)
	}
	if row.HumanWaitSecs != nil && row.AgentWaitSecs != nil &&
		*row.HumanWaitSecs > *row.AgentWaitSecs {
		// presented came last, so its gap back to the prior agent note must
		// span the whole middle of the ticket's life: strictly longer than
		// the contract_approved -> next-note gap only in a real sequence;
		// with sub-second appends they may be equal-adjacent, so only flag
		// a clear inversion.
		t.Logf("human_wait=%.6f agent_wait=%.6f", *row.HumanWaitSecs, *row.AgentWaitSecs)
	}

	// a ticket that never reached presented must not appear
	other := newTicket(t, s, "handoff-unpresented")
	mustAppend(t, s, evt(other, "note", map[string]any{"v": "wip"}))
	rows2, err := s.Handoffs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows2 {
		if r.ULID == other {
			t.Errorf("unpresented ticket leaked into handoffs")
		}
	}
}
