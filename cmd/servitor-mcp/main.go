// servitor-mcp: exposes the Service as MCP tools over stdio.
// Point your MCP client at this binary; it needs SERVITOR_DSN.
package main

import (
	"context"
	"log"
	"os"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/mcp"
	"github.com/prestontallen/servitor/internal/store"
)

func main() {
	dsn := os.Getenv("SERVITOR_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:psql@localhost:5432/servitor?sslmode=disable"
	}
	ctx := context.Background()
	s, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("store open: %v", err)
	}
	srv := &mcp.Server{
		Service: api.NewStoreService(s),
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
	}
	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("mcp serve: %v", err)
	}
}
