package api

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prestontallen/servitor/internal/store"
)

// storeForTest provisions a throwaway database per test, mirroring the
// store package's approach.
func storeForTest(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, "postgres://postgres:psql@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	defer admin.Close(ctx)
	for _, q := range []string{
		`DROP DATABASE IF EXISTS servitor_api_test WITH (FORCE)`,
		`CREATE DATABASE servitor_api_test`,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	dsn := os.Getenv("SERVITOR_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:psql@localhost:5432/servitor_api_test?sslmode=disable"
	}
	s, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySchema(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Pool.Close() })
	return s
}
