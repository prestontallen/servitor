// servitord: the HTTP API daemon. The store is the only backend; this
// process is stateless and swappable.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

func main() {
	// servitord apply-schema: create/repair the schema on SERVITOR_DSN, then exit.
	if len(os.Args) > 1 && os.Args[1] == "apply-schema" {
		dsn := os.Getenv("SERVITOR_DSN")
		if dsn == "" {
			dsn = "postgres://postgres:psql@localhost:5432/servitor?sslmode=disable"
		}
		ctx := context.Background()
		s, err := store.Open(ctx, dsn)
		if err != nil {
			log.Fatalf("store open: %v", err)
		}
		if err := s.ApplySchema(ctx); err != nil {
			log.Fatalf("apply schema: %v", err)
		}
		log.Println("schema applied")
		return
	}

	dsn := os.Getenv("SERVITOR_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:psql@localhost:5432/servitor?sslmode=disable"
	}
	addr := os.Getenv("SERVITOR_ADDR")
	if addr == "" {
		addr = ":8181"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("store open: %v", err)
	}

	svc := api.NewStoreService(s)
	log.Printf("servitord listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, api.NewHTTP(svc).Routes()))
}
