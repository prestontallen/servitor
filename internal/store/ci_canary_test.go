package store

import "testing"

// Temporary, ticket ci-db-tests: these two prove the CI wiring. One fails
// only when a database is reachable (so a red run proves the service is
// there and failures surface); one skips (so a red run proves the skip
// gate names it). Both are removed in the next commit on the branch.

func TestCICanaryFails(t *testing.T) {
	s := testDB(t)
	var n int
	if err := s.Pool.QueryRow(t.Context(), `SELECT count(*) FROM tickets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("ci canary: the database is reachable (%d tickets) and this failure must make the job red", n)
}

func TestCICanarySkips(t *testing.T) {
	t.Skip("ci canary: the skip gate must name this test and make the job red")
}
