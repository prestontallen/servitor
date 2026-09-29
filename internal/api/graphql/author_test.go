package graphql

import (
	"context"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
)

// TestGraphQLAuthorFields: events carry host and session, and a ticket
// carries active_host from the event that set it active.
func TestGraphQLAuthorFields(t *testing.T) {
	svc := testService(t)
	id := create(t, svc, "gq-author")
	if _, err := svc.Append(context.Background(), api.WriteCmd{Ticket: id, Kind: "field.set", Actor: "agent:claude",
		Payload: map[string]any{"field": "tier", "v": 1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Append(context.Background(), api.WriteCmd{Ticket: id, Kind: "status.set", Actor: "agent:claude",
		Host: "adirondack", Session: "S", Payload: map[string]any{"status": "active"}}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Ticket struct {
			ActiveBy   string `json:"active_by"`
			ActiveHost string `json:"active_host"`
		} `json:"ticket"`
		Events struct {
			Events []struct {
				Host    *string `json:"host"`
				Session *string `json:"session"`
			} `json:"events"`
		} `json:"events"`
	}
	mustData(t, query(t, svc, `{ ticket(ref: "gq-author") { active_by active_host }
		events(tickets: ["gq-author"]) { events { host session } } }`, nil), &got)
	if got.Ticket.ActiveBy != "agent:claude" || got.Ticket.ActiveHost != "adirondack" {
		t.Errorf("ticket: %+v", got.Ticket)
	}
	evs := got.Events.Events
	if len(evs) != 2 {
		t.Fatalf("events: %d", len(evs))
	}
	// newest first: the stamped status.set, then the bare create
	if evs[0].Host == nil || *evs[0].Host != "adirondack" || evs[0].Session == nil || *evs[0].Session != "S" {
		t.Errorf("stamped event: host %v session %v", evs[0].Host, evs[0].Session)
	}
	if evs[1].Host != nil {
		t.Errorf("bare event host %v, want null", *evs[1].Host)
	}
}
