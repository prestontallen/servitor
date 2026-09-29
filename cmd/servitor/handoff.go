package main

import (
	"fmt"
	"strings"

	"github.com/prestontallen/servitor/internal/api"
)

// ctxAuthor is one entry of ctx's authors list (newest first).
type ctxAuthor struct {
	Actor   string  `json:"actor"`
	Host    *string `json:"host"`
	Session string  `json:"session"`
	LastTS  string  `json:"last_ts"`
}

// handoffLine compares the reader with the ticket's last author and says
// what kind of pickup this is, so an agent knows before it acts whether it
// is continuing its own work, taking over on this machine, or looking at
// work that lives somewhere else. "" when the ticket has no authors.
// pushed is the ticket's pushed field as ctx returns it (bool or string).
func handoffLine(self api.Author, authors []ctxAuthor, pushed any) string {
	if len(authors) == 0 {
		return ""
	}
	last := authors[0]
	host, session := "unknown host", "no session"
	if last.Host != nil {
		host = *last.Host
	}
	if last.Session != "" {
		session = "session " + last.Session
	}
	who := fmt.Sprintf("last %s on %s, %s, %s", last.Actor, host, session, last.LastTS)

	var verdict string
	switch {
	case strings.HasPrefix(last.Actor, "human:"):
		verdict = "human wrote last: read their events first, they outrank the plan"
	case self.Session != "" && last.Session == self.Session:
		verdict = "same session: continuing your own work"
	case last.Host == nil:
		verdict = "last author predates host records: check branch and pushed before trusting the worktree"
	case *last.Host != self.Host:
		verdict = fmt.Sprintf("other host %s: the worktree and any unpushed commits live there", *last.Host)
		if fmt.Sprint(pushed) == "false" {
			verdict += fmt.Sprintf("; pushed=false, so the work is stranded on %s until it is pushed", *last.Host)
		}
	default:
		verdict = "new session on this host: reread the ledger; the worktree and commits are on this disk"
	}
	if last.Actor != self.Actor && !strings.HasPrefix(last.Actor, "human:") {
		verdict = fmt.Sprintf("other agent (%s), ", last.Actor) + verdict
	}
	return "handoff author: " + verdict + " (" + who + ")"
}
