// servitor CLI: a thin client over the API. It holds no state and applies
// no rules — every fact comes from the Service, every write is an event.
//
// The SessionStart hook contract: `servitor ctx` ALWAYS exits 0, degrading
// to one line when the API is unreachable. Everything else exits non-zero
// on failure.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prestontallen/servitor/internal/api"
	"github.com/prestontallen/servitor/internal/store"
)

func main() {
	c := &api.HTTPClient{
		Base:  envOr("SERVITOR_API", "http://localhost:8181"),
		Actor: envOr("SERVITOR_ACTOR", "agent:cli"),
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, c, os.Getenv))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// run executes one command and returns the process exit code. All I/O goes
// through the writers; all failure paths return codes instead of exiting.
func run(args []string, stdout, stderr io.Writer, c *api.HTTPClient, env func(string) string) int {
	ctx := context.Background()
	say := func(format string, a ...any) { fmt.Fprintf(stderr, "servitor: "+format+"\n", a...) }
	if len(args) < 1 {
		usage(stderr)
		return 2
	}
	cmd, args := args[0], args[1:]

	switch cmd {
	case "hook", "ctx":
		// SessionStart hook: never block a session.
		ref := ""
		if len(args) > 0 {
			ref = args[0]
		} else if v := env("SERVITOR_TICKET"); v != "" {
			ref = v
		}
		if ref == "" {
			board, err := c.Board(ctx)
			if err != nil {
				fmt.Fprintf(stdout, "servitor: unavailable (%v)\n", err) // one line, exit 0
				return 0
			}
			fmt.Fprintf(stdout, "servitor: no focused ticket (set SERVITOR_TICKET). %d open card(s):\n", len(board))
			for _, c := range board {
				fmt.Fprintf(stdout, "  [%s] %s (%s)\n", c.Status, c.Slug, c.ULID[:8])
			}
			return 0
		}
		doc, err := c.Ctx(ctx, ref)
		if err != nil {
			fmt.Fprintf(stdout, "servitor: unavailable (%v)\n", err) // one line, exit 0
			return 0
		}
		stdout.Write(doc)
		fmt.Fprintln(stdout)
		return 0

	case "board":
		cards, err := c.Board(ctx)
		if err != nil {
			say("%v", err)
			return 1
		}
		return encode(stdout, cards)

	case "list":
		// servitor list [--status S]... [--query Q] [--limit N]
		// Reaches done and dropped; no flags = every ticket, arcs included.
		f := store.ListFilter{}
		for i := 0; i < len(args); i += 2 {
			if i+1 >= len(args) {
				say("list: flag %s needs a value", args[i])
				return 2
			}
			switch args[i] {
			case "--status":
				f.Statuses = append(f.Statuses, strings.ToLower(args[i+1]))
			case "--query":
				f.Query = args[i+1]
			case "--limit":
				n, err := strconv.Atoi(args[i+1])
				if err != nil || n <= 0 {
					say("list: --limit must be a positive integer")
					return 2
				}
				f.Limit = n
			default:
				say("list: unknown flag %s", args[i])
				return 2
			}
		}
		cards, err := c.List(ctx, f)
		if err != nil {
			say("%v", err)
			return 1
		}
		return encode(stdout, cards)

	case "arcs":
		arcs, err := c.Arcs(ctx)
		if err != nil {
			say("%v", err)
			return 1
		}
		return encode(stdout, arcs)

	case "new":
		var slug, title string
		rank := "0"
		for i := 0; i < len(args); i += 2 {
			if i+1 >= len(args) {
				say("new: flag %s needs a value", args[i])
				return 2
			}
			switch args[i] {
			case "--slug":
				slug = args[i+1]
			case "--title":
				title = args[i+1]
			case "--rank":
				rank = args[i+1]
			default:
				say("new: unknown flag %s", args[i])
				return 2
			}
		}
		if slug == "" {
			say("new: --slug is required")
			return 2
		}
		res, err := c.Append(ctx, api.WriteCmd{
			Kind:    "ticket.create",
			Actor:   c.Actor,
			Payload: map[string]any{"slug": slug, "title": title, "rank": json.Number(rank)},
		})
		if err != nil {
			say("%v", err)
			return 1
		}
		fmt.Fprintln(stdout, res.TicketULID)
		return 0

	case "set":
		// servitor set <ref> [--status S [--on WHO]] [--pr VALUE|-] [field=value ...]
		// "-" as a value means absent (key removed / NULL).
		if len(args) < 2 {
			say("set: need a ref and at least one change")
			return 2
		}
		ref := args[0]
		var cmds []api.WriteCmd
		i := 1
		for i < len(args) {
			switch {
			case args[i] == "--status":
				if i+1 >= len(args) {
					say("set: --status needs a value")
					return 2
				}
				p := map[string]any{"status": args[i+1]}
				if i+2 < len(args) && args[i+2] == "--on" {
					p["on"] = args[i+3]
					i += 2
				}
				cmds = append(cmds, api.WriteCmd{Ticket: ref, Kind: "status.set", Payload: p})
				i += 2
			case args[i] == "--pr":
				if i+1 >= len(args) {
					say("set: --pr needs a value")
					return 2
				}
				p := map[string]any{"field": "pr"}
				if args[i+1] != "-" {
					p["v"] = args[i+1]
				}
				cmds = append(cmds, api.WriteCmd{Ticket: ref, Kind: "field.set", Payload: p})
				i += 2
			case strings.Contains(args[i], "="):
				kv := strings.SplitN(args[i], "=", 2)
				p := map[string]any{"field": kv[0]}
				if kv[1] != "-" {
					p["v"] = jsonValue(kv[1])
				}
				cmds = append(cmds, api.WriteCmd{Ticket: ref, Kind: "field.set", Payload: p})
				i++
			default:
				say("set: cannot parse %q", args[i])
				return 2
			}
		}
		var last int64
		for _, cc := range cmds {
			cc.Actor = c.Actor
			res, err := c.Append(ctx, cc)
			if err != nil {
				say("%v", err)
				return 1
			}
			last = res.EventID
		}
		fmt.Fprintln(stdout, last)
		return 0

	case "log":
		if len(args) < 2 {
			say("log: need <ref> <kind> [text]")
			return 2
		}
		text := ""
		if len(args) > 2 {
			text = strings.Join(args[2:], " ")
		}
		// a JSON object body is used verbatim as the payload (structured
		// kinds like feedback); anything else is wrapped as {"v": text}
		payload := map[string]any{"v": text}
		if strings.HasPrefix(strings.TrimSpace(text), "{") {
			var obj map[string]any
			if err := json.Unmarshal([]byte(text), &obj); err == nil {
				payload = obj
			}
		}
		res, err := c.Append(ctx, api.WriteCmd{
			Ticket: args[0], Kind: args[1], Actor: c.Actor,
			Payload: payload,
		})
		if err != nil {
			say("%v", err)
			return 1
		}
		fmt.Fprintln(stdout, res.EventID)
		return 0

	case "gate":
		if len(args) < 2 {
			say("gate: need <ref> <contract_approved|presented|shipped>")
			return 2
		}
		actor := c.Actor
		if args[1] == "contract_approved" && !strings.HasPrefix(actor, "human:") {
			if h := env("SERVITOR_HUMAN"); h != "" {
				actor = "human:" + h
			}
		}
		res, err := c.Append(ctx, api.WriteCmd{
			Ticket: args[0], Kind: "gate", Actor: actor,
			Payload: map[string]any{"gate": args[1]},
		})
		if err != nil {
			say("%v", err)
			return 1
		}
		fmt.Fprintln(stdout, res.EventID)
		return 0

	case "history":
		if len(args) < 1 {
			say("history: need <ref>")
			return 2
		}
		evs, err := c.History(ctx, args[0], 10000)
		if err != nil {
			say("%v", err)
			return 1
		}
		return encode(stdout, evs)

	case "feedback":
		f := api.FeedbackFilter{}
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--since":
				i++
				if i < len(args) {
					for _, layout := range []string{"2006-01-02", time.RFC3339} {
						if t, err := time.Parse(layout, args[i]); err == nil {
							f.Since = &t
							break
						}
					}
				}
			case "--source":
				i++
				if i < len(args) {
					f.Source = args[i]
				}
			case "--limit":
				i++
				if i < len(args) {
					fmt.Sscanf(args[i], "%d", &f.Limit)
				}
			}
		}
		evs, err := c.Feedback(ctx, f)
		if err != nil {
			say("%v", err)
			return 1
		}
		return encode(stdout, evs)

	default:
		usage(stderr)
		return 2
	}
}

func encode(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "servitor: %v\n", err)
		return 1
	}
	return 0
}

// jsonValue passes JSON through when it looks like JSON, else treats it as a
// string — CLI ergonomics for `servitor set x priority=high`.
func jsonValue(s string) any {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		var v any
		if json.Unmarshal([]byte(t), &v) == nil {
			return v
		}
	}
	return s
}

func usage(w io.Writer) {
	fmt.Fprint(w, `servitor — thin client over the servitor API

  ctx [ref]        whole ticket aggregate. ALWAYS exits 0 (hook contract).
  board            queued/active/blocked cards
  list [--status S]... [--query Q] [--limit N]
                                   all tickets incl. done/dropped (arcs too)
  arcs             arcs (tickets with members) with derived rollups
  new --slug S [--title T] [--rank N]
  set <ref> [--status S [--on WHO]] [--pr V|-] [field=value ...]
  log <ref> <kind> [text]          note, or any ledger kind (JSON object = payload)
  feedback [--since DATE] [--source human|self] [--limit N]
                                   feedback events across all tickets
  gate <ref> <gate>                contract_approved requires SERVITOR_HUMAN
  history <ref>                    full event timeline

env: SERVITOR_API (default http://localhost:8181), SERVITOR_ACTOR (default agent:cli),
     SERVITOR_HUMAN (your human id, for gates), SERVITOR_TICKET (hook focus)
`)
}
