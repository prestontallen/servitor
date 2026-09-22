package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// Shapes under test are documented in skills/servitor/references/events.md.

func TestContractAndReviewValidation(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "pr-validate")

	_, err := s.AppendEvent(ctx, evt(id, "contract", map[string]any{"in": []string{"x"}}), time.Time{})
	expectFail(t, "contract without intent", err, "intent")
	_, err = s.AppendEvent(ctx, evt(id, "review", map[string]any{"verdict": "maybe"}), time.Time{})
	expectFail(t, "review with bad verdict", err, "verdict")
	_, err = s.AppendEvent(ctx, evt(id, "review", map[string]any{"summary": "no verdict"}), time.Time{})
	expectFail(t, "review without verdict", err, "verdict")

	mustAppend(t, s, evt(id, "contract", map[string]any{"intent": "ok"}))
	mustAppend(t, s, evt(id, "review", map[string]any{"verdict": "present"}))
}

func TestCtxReadContractReviewEvidence(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "pr-ctx")
	crit := addSubitem(t, s, id, "criterion", "when X then Y")
	addSubitem(t, s, id, "criterion", "when P then Q")

	mustAppend(t, s, evt(id, "subitem.set", map[string]any{"ulid": crit[:12], "state": "pass", "evidence": "go test -run TestX"}))
	mustAppend(t, s, evt(id, "contract", map[string]any{
		"intent": "v1", "in": []string{"a"}, "out": []string{"b"}, "verification": "staging", "risks": "none"}))
	mustAppend(t, s, evt(id, "contract", map[string]any{"intent": "v2", "in": []string{"a", "c"}}))
	mustAppend(t, s, evt(id, "review", map[string]any{"verdict": "hold", "summary": "1/2", "findings": []any{
		map[string]any{"loc": "f.go:L1", "tag": "scope", "what": "w", "fix": "f"}}}))
	mustAppend(t, s, evt(id, "review", map[string]any{"verdict": "present", "summary": "2/2", "findings": []any{}}))

	raw, err := s.CtxRead(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Criteria []struct {
			ULID     string  `json:"ulid"`
			State    *string `json:"state"`
			Evidence *string `json:"evidence"`
		} `json:"criteria"`
		Contract struct {
			Intent string   `json:"intent"`
			In     []string `json:"in"`
			Actor  string   `json:"actor"`
			TS     string   `json:"ts"`
		} `json:"contract"`
		Review struct {
			Verdict  string `json:"verdict"`
			Summary  string `json:"summary"`
			Runs     int    `json:"runs"`
			Actor    string `json:"actor"`
			Findings []any  `json:"findings"`
		} `json:"review"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Criteria) != 2 {
		t.Fatalf("criteria: %+v", doc.Criteria)
	}
	for _, c := range doc.Criteria {
		if c.ULID == crit {
			if c.Evidence == nil || *c.Evidence != "go test -run TestX" {
				t.Errorf("proven criterion evidence: %v", c.Evidence)
			}
		} else if c.Evidence != nil {
			t.Errorf("unproven criterion must have null evidence, got %q", *c.Evidence)
		}
	}
	if doc.Contract.Intent != "v2" || len(doc.Contract.In) != 2 || doc.Contract.Actor == "" || doc.Contract.TS == "" {
		t.Errorf("contract: latest wins with actor/ts, got %+v", doc.Contract)
	}
	if doc.Review.Verdict != "present" || doc.Review.Runs != 2 || doc.Review.Summary != "2/2" || doc.Review.Actor == "" {
		t.Errorf("review: latest wins with runs=2, got %+v", doc.Review)
	}
	if doc.Review.Findings == nil || len(doc.Review.Findings) != 0 {
		t.Errorf("empty findings must round-trip as [], got %v", doc.Review.Findings)
	}

	// a ticket with neither has null contract and review, not missing keys
	bare := newTicket(t, s, "pr-bare")
	raw, err = s.CtxRead(ctx, bare)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"contract", "review"} {
		if v, ok := m[k]; !ok || v != nil {
			t.Errorf("bare ticket %s: want explicit null, got %v (present=%v)", k, v, ok)
		}
	}
}

func TestBoardPlanMark(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	id := newTicket(t, s, "pr-board")
	a := addSubitem(t, s, id, "criterion", "a")
	addSubitem(t, s, id, "criterion", "b")
	addSubitem(t, s, id, "criterion", "c")
	addSubitem(t, s, id, "plan", "not a criterion")
	mustAppend(t, s, evt(id, "subitem.set", map[string]any{"ulid": a[:12], "state": "pass"}))
	bare := newTicket(t, s, "pr-board-bare")

	cards, err := s.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Card{}
	for _, c := range cards {
		byID[c.ULID] = c
	}
	c := byID[id]
	if c.CriteriaPass != 1 || c.CriteriaTotal != 3 {
		t.Errorf("plan mark: pass=%d total=%d, want 1/3", c.CriteriaPass, c.CriteriaTotal)
	}
	b := byID[bare]
	if b.CriteriaPass != 0 || b.CriteriaTotal != 0 {
		t.Errorf("bare card: pass=%d total=%d, want 0/0", b.CriteriaPass, b.CriteriaTotal)
	}
	// List shares the column list; it must scan the new columns too.
	if _, err := s.List(ctx, ListFilter{}); err != nil {
		t.Fatalf("List after board columns: %v", err)
	}
}
