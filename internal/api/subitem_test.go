package api

import (
	"context"
	"testing"
)

// addSubitem appends a subitem.add and returns the result plus the ledger
// payload's ulid and the projected row's ulid, read back independently.
func addSubitem(t *testing.T, s Service, ticket string, payload map[string]any) (res AppendResult, ledgerULID, rowULID string) {
	t.Helper()
	ctx := context.Background()
	res, err := s.Append(ctx, WriteCmd{Ticket: ticket, Kind: "subitem.add", Actor: "agent:test", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	st := s.(*StoreService).Store
	if err := st.Pool.QueryRow(ctx, `SELECT payload->>'ulid' FROM ledger WHERE id=$1`, res.EventID).Scan(&ledgerULID); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `SELECT ulid FROM subitems WHERE ticket_ulid=$1 AND body=$2`, ticket, payload["body"]).Scan(&rowULID); err != nil {
		t.Fatal(err)
	}
	return res, ledgerULID, rowULID
}

func TestSubitemAddMintsAndReturnsULID(t *testing.T) {
	s := svc(t)
	id := mustCreate(t, s, "sub-mint")
	payload := map[string]any{"kind": "finding", "body": "src=self x.go:L1: check: none. Add one.", "rank": 0}
	res, ledger, row := addSubitem(t, s, id, payload)
	if res.SubitemULID == "" {
		t.Fatal("append result carries no subitem_ulid")
	}
	if ledger != res.SubitemULID || row != res.SubitemULID {
		t.Errorf("result %s, ledger payload %s, row %s: want all equal", res.SubitemULID, ledger, row)
	}
	if _, ok := payload["ulid"]; ok {
		t.Error("the caller's payload map was written")
	}
	// the returned handle is what subitem.set addresses
	if _, err := s.Append(context.Background(), WriteCmd{Ticket: id, Kind: "subitem.set", Actor: "agent:test",
		Payload: map[string]any{"ulid": res.SubitemULID[:12], "state": "applied"}}); err != nil {
		t.Fatalf("closing by the returned prefix: %v", err)
	}
}

func TestSubitemAddKeepsSuppliedULID(t *testing.T) {
	s := svc(t)
	id := mustCreate(t, s, "sub-keep")
	const given = "01M3N0AAAAAAAAAAAAAAAAAAAA"
	res, ledger, row := addSubitem(t, s, id, map[string]any{"ulid": given, "kind": "criterion", "body": "when x then y", "rank": 0})
	if res.SubitemULID != given || ledger != given || row != given {
		t.Errorf("result %s, ledger %s, row %s: want %s everywhere", res.SubitemULID, ledger, row, given)
	}
}

func TestNonSubitemAppendHasNoSubitemULID(t *testing.T) {
	s := svc(t)
	id := mustCreate(t, s, "sub-none")
	res, err := s.Append(context.Background(), WriteCmd{Ticket: id, Kind: "note", Actor: "agent:test", Payload: map[string]any{"v": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.SubitemULID != "" {
		t.Errorf("note append returned subitem_ulid %q", res.SubitemULID)
	}
}
