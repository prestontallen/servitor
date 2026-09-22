// Package testdb supplies the superuser connection the database-backed tests
// use to create and drop their throwaway databases.
//
// It exists because that DSN used to be a literal in each test file: one admin
// password, spelled out in four files, and therefore readable in git history on
// every clone and on GitHub. It now comes from the environment, and an unset
// variable skips the test — which is the same outcome these tests already had
// whenever the database was unreachable, so nothing that used to run stops
// running for anyone who sets it.
package testdb

import (
	"net/url"
	"os"
	"testing"
)

// EnvVar names the superuser DSN, for example
//
//	postgres://postgres:PASSWORD@localhost:5432/postgres?sslmode=disable
const EnvVar = "SERVITOR_TEST_ADMIN_DSN"

// AdminDSN returns the superuser DSN, skipping the test when it is unset.
func AdminDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv(EnvVar)
	if dsn == "" {
		t.Skipf("%s is not set; these tests need a superuser DSN to create throwaway databases", EnvVar)
	}
	return dsn
}

// Named points a DSN at a different database. Each package derives its own
// throwaway database from the one credential this way, rather than spelling a
// second DSN out — which is how the password came to be duplicated in the first
// place.
func Named(t *testing.T, dsn, database string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("%s is not a valid URL: %v", EnvVar, err)
	}
	u.Path = "/" + database
	return u.String()
}
