// servitord: the HTTP API daemon. The store is the only backend; this
// process is stateless and swappable.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

// usage is written to w. Kept short: the daemon takes no flags, and the two
// things a reader needs are the one subcommand and the environment it reads.
func usage(w io.Writer) {
	fmt.Fprint(w, `servitord — the servitor HTTP API daemon.

Usage:
  servitord                 serve the API (default)
  servitord apply-schema    create/repair the schema on SERVITOR_DSN, then exit
  servitord --help          print this and exit

Environment:
  SERVITOR_DSN     Postgres/TimescaleDB DSN         (default: local servitor DB)
  SERVITOR_ADDR    listen address                   (default: :8181)
  SERVITOR_TOKEN   bearer token for the API         (default: off, no auth)
`)
}

func main() {
	// Argument handling comes before anything that opens a socket. Every
	// argument except apply-schema used to fall through to ListenAndServe, so
	// `servitord --help` started the daemon instead of describing it — and a
	// typo like `--hlep` did the same, silently.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			usage(os.Stdout)
			return
		case "apply-schema":
			// handled below
		default:
			fmt.Fprintf(os.Stderr, "servitord: unknown argument %q\n\n", os.Args[1])
			usage(os.Stderr)
			os.Exit(2)
		}
	}

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
	h := api.NewHTTP(svc)
	if tok := os.Getenv("SERVITOR_TOKEN"); tok != "" {
		h.Token = tok
		log.Printf("auth enabled (bearer token)")
	}
	log.Printf("servitord listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, h.Routes()))
}
