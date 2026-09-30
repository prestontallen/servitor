package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
)

// TestNewRequiresTier: servitor new classifies the ticket in the event that
// creates it. Without --tier, or with one the store would refuse, the CLI
// exits 2 before any write and no ticket with that slug exists.
func TestNewRequiresTier(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()

	out, code := cli(t, svc, nil, "new", "--slug", "nt-ok", "--tier", "2", "--title", "classified")
	if code != 0 {
		t.Fatalf("new --tier 2 exited %d: %s", code, out)
	}
	raw, err := svc.Ctx(ctx, "nt-ok")
	if err != nil {
		t.Fatal(err)
	}
	var agg struct {
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(raw, &agg); err != nil {
		t.Fatal(err)
	}
	if got := agg.Fields["tier"]; got != float64(2) {
		t.Fatalf("fields.tier after new --tier 2 = %#v, want 2", got)
	}
	evs, err := svc.History(ctx, "nt-ok", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Kind != "ticket.create" {
		t.Fatalf("new --tier wrote %d events, want exactly one ticket.create: %+v", len(evs), evs)
	}

	// no --tier, then each shape the store refuses: nothing is written
	bad := [][]string{
		{"new", "--slug", "nt-none"},
		{"new", "--slug", "nt-four", "--tier", "4"},
		{"new", "--slug", "nt-word", "--tier", "two"},
		{"new", "--slug", "nt-frac", "--tier", "1.5"},
		{"new", "--slug", "nt-empty", "--tier", ""},
	}
	for _, args := range bad {
		out, code := cli(t, svc, nil, args...)
		if code != 2 || !strings.Contains(out, "--tier") {
			t.Fatalf("%v: exit %d, output %q; want exit 2 naming --tier", args, code, out)
		}
		if _, err := svc.Ctx(ctx, args[2]); err == nil {
			t.Fatalf("%v: a ticket %s exists after a refused new", args, args[2])
		}
	}
}
