package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPolicyCodesOverHTTP: each rule of the live write path answers 422
// with its code, and the human waive lands with 201.
func TestPolicyCodesOverHTTP(t *testing.T) {
	s := svc(t)
	ctx := context.Background()
	id := mustCreate(t, s, "policy-http")
	h := NewHTTP(s).Routes()
	post := func(body, actorHeader string) (int, string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
		if actorHeader != "" {
			req.Header.Set("X-Servitor-Actor", actorHeader)
		}
		h.ServeHTTP(rec, req)
		var er struct {
			Error struct{ Code string }
		}
		json.Unmarshal(rec.Body.Bytes(), &er)
		return rec.Code, er.Error.Code
	}
	if code, ec := post(`{"ticket":"policy-http","kind":"status.set","actor":"agent:test","payload":{"status":"active"}}`, ""); code != 422 || ec != "tier_required" {
		t.Fatalf("active without tier: %d %q", code, ec)
	}
	if _, err := s.Append(ctx, WriteCmd{Ticket: id, Kind: "field.set", Actor: "agent:test", Payload: map[string]any{"field": "tier", "v": 2}}); err != nil {
		t.Fatal(err)
	}
	if code, ec := post(`{"ticket":"policy-http","kind":"field.set","actor":"agent:test","payload":{"field":"head","v":"abc1234"}}`, ""); code != 422 || ec != "contract_required" {
		t.Fatalf("head before the gate: %d %q", code, ec)
	}
	if _, err := s.Append(ctx, WriteCmd{Ticket: id, Kind: "subitem.add", Actor: "agent:test", Payload: map[string]any{"kind": "criterion", "body": "unproven"}}); err != nil {
		t.Fatal(err)
	}
	if code, ec := post(`{"ticket":"policy-http","kind":"status.set","actor":"agent:test","payload":{"status":"done"}}`, ""); code != 422 || ec != "criteria_incomplete" {
		t.Fatalf("done with an unproven criterion: %d %q", code, ec)
	}
	if code, ec := post(`{"ticket":"policy-http","kind":"status.set","actor":"agent:test","payload":{"status":"done","waive":"agent says so"}}`, ""); code != 422 || ec != "human_waiver_required" {
		t.Fatalf("agent waive: %d %q", code, ec)
	}
	if code, ec := post(`{"ticket":"policy-http","kind":"status.set","actor":"agent:test","payload":{"status":"done","waive":"accepted"}}`, "human:preston"); code != 201 {
		t.Fatalf("human waive: %d %q", code, ec)
	}
}
