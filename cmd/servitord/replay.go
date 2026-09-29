package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/prestontallen/servitor/internal/store"
)

func dsnFromEnv() string {
	if dsn := os.Getenv("SERVITOR_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://postgres:psql@localhost:5432/servitor?sslmode=disable"
}

// replay runs `servitord replay [--dry-run] [--lossy]` and returns the exit
// code: 0 on a completed (or rolled-back dry) run, 1 on refusal or error, 2
// on a bad argument. The report is one line, plus one per skipped row.
func replay(args []string, stdout, stderr io.Writer) int {
	var opts store.ReplayOptions
	for _, a := range args {
		switch a {
		case "--dry-run":
			opts.DryRun = true
		case "--lossy":
			opts.Lossy = true
		default:
			fmt.Fprintf(stderr, "servitord replay: unknown argument %q\n\n", a)
			usage(stderr)
			return 2
		}
	}
	ctx := context.Background()
	s, err := store.Open(ctx, dsnFromEnv())
	if err != nil {
		fmt.Fprintf(stderr, "store open: %v\n", err)
		return 1
	}
	defer s.Pool.Close()
	rep, err := s.Replay(ctx, opts)
	if err != nil {
		fmt.Fprintf(stderr, "replay refused, nothing changed: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, formatReport(rep))
	return 0
}

func formatReport(rep store.ReplayReport) string {
	head := "replay"
	if rep.DryRun {
		head = "dry run (rolled back)"
	}
	out := fmt.Sprintf("%s: %d rows re-applied, %d sub-item identities recovered, %d minted fresh, %d rows skipped; watermark %d\n",
		head, rep.Rows, rep.Recovered, rep.Fresh, len(rep.Skipped), rep.Watermark)
	for _, r := range rep.Skipped {
		out += fmt.Sprintf("  skipped event %d (%s): %s\n", r.ID, r.Kind, r.Reason)
	}
	return out
}
