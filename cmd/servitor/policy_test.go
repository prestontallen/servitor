package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// runAs is runWith with a chosen actor: the waive is a human's call.
func runAs(actor string, env map[string]string, args ...string) (string, int) {
	c := &api.HTTPClient{Base: env["SERVITOR_API"], Actor: actor}
	var out bytes.Buffer
	code := run(args, &out, &out, c, func(k string) string { return env[k] })
	return out.String(), code
}

func policyTicket(t *testing.T, svc api.Service, slug string) string {
	t.Helper()
	id := store.NewULID()
	if _, err := svc.Append(context.Background(), api.WriteCmd{
		Ticket: id, Kind: "ticket.create", Actor: "agent:test",
		Payload: map[string]any{"slug": slug, "title": "T"},
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestSetFieldsBeforeStatus: the tier after the status on the command line
// still reaches the store first; the reverse order works too; and without
// a tier the CLI surfaces the store's code.
func TestSetFieldsBeforeStatus(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	policyTicket(t, svc, "order-a")
	policyTicket(t, svc, "order-b")
	policyTicket(t, svc, "order-none")
	if out, code := cli(t, svc, nil, "set", "order-a", "--status", "active", "tier=2"); code != 0 {
		t.Fatalf("status then tier exited %d: %s", code, out)
	}
	if out, code := cli(t, svc, nil, "set", "order-b", "tier=0", "--status", "active"); code != 0 {
		t.Fatalf("tier then status exited %d: %s", code, out)
	}
	out, code := cli(t, svc, nil, "set", "order-none", "--status", "active")
	if code == 0 || !strings.Contains(out, "tier_required") {
		t.Fatalf("active without tier: exit %d, output %q, want tier_required", code, out)
	}
}

// TestSetWaive: --waive rides in the status.set payload; an agent is
// refused with the code, a human lands it and history shows the reason.
func TestSetWaive(t *testing.T) {
	st := apiTestStore(t)
	svc := api.NewStoreService(st)
	ctx := context.Background()
	id := policyTicket(t, svc, "waive-cli")
	if _, err := svc.Append(ctx, api.WriteCmd{Ticket: id, Kind: "subitem.add", Actor: "agent:test",
		Payload: map[string]any{"kind": "criterion", "body": "unproven"}}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.NewHTTP(svc).Routes())
	t.Cleanup(srv.Close)
	env := map[string]string{"SERVITOR_API": srv.URL}

	out, code := runAs("agent:test", env, "set", "waive-cli", "--status", "done", "--waive", "agent says so")
	if code == 0 || !strings.Contains(out, "human_waiver_required") {
		t.Fatalf("agent waive: exit %d, output %q, want human_waiver_required", code, out)
	}
	out, code = runAs("agent:test", env, "set", "waive-cli", "--status", "done")
	if code == 0 || !strings.Contains(out, "criteria_incomplete") {
		t.Fatalf("done without waive: exit %d, output %q, want criteria_incomplete", code, out)
	}
	if out, code := runAs("human:preston", env, "set", "waive-cli", "--status", "done", "--waive", "accepted the local proof"); code != 0 {
		t.Fatalf("human waive exited %d: %s", code, out)
	}
	evs, err := svc.History(ctx, id, 100)
	if err != nil {
		t.Fatal(err)
	}
	last := evs[len(evs)-1]
	if last.Kind != "status.set" || last.Actor != "human:preston" || last.Payload["waive"] != "accepted the local proof" {
		t.Fatalf("history does not carry the waive: %+v", last)
	}
}
