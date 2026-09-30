package api

import (
	"os"
	"strings"
)

// Author is who wrote an event (actor), from which machine (host) and in
// which harness session (session). Each part comes from the layer that
// knows it: the harness exports its session id to every command it runs,
// and the OS knows the host. Nobody has to remember to declare them.
type Author struct {
	Actor   string
	Host    string
	Session string
}

// harnesses are checked innermost first: Hermes launched from a Claude Code
// shell inherits CLAUDE_CODE_SESSION_ID, so its own id must win. The reverse
// nesting (Claude Code started from a Hermes terminal) would be misread as
// Hermes; revisit if that ever happens in practice.
//
// Cursor comes last: CURSOR_AGENT marks the agent's shells but carries no
// session (Cursor gives the conversation id to hook processes only), so any
// real session var in the same environment — Claude Code run from Cursor's
// terminal — must win. Its session stays "".
var harnesses = []struct {
	sessionVar, actor string
	marker            bool // the var only marks the harness; its value is no session
}{
	{"HERMES_SESSION_ID", "agent:hermes", false},
	{"CLAUDE_CODE_SESSION_ID", "agent:claude", false},
	{"CURSOR_AGENT", "agent:cursor", true},
}

// LocalAuthor derives the author of writes from this process. SERVITOR_ACTOR
// still wins for the actor (a human gate, a manual override); the session
// and host are stamped either way. With no harness the actor is agent:cli.
// SERVITOR_HOST overrides the hostname, for containers whose hostname is a
// random id.
func LocalAuthor(env func(string) string) Author {
	a := Author{Actor: "agent:cli", Host: NormHost(env("SERVITOR_HOST"))}
	if a.Host == "" {
		h, _ := os.Hostname()
		a.Host = NormHost(h)
	}
	for _, h := range harnesses {
		if s := env(h.sessionVar); s != "" {
			a.Actor = h.actor
			if !h.marker {
				a.Session = s
			}
			break
		}
	}
	if v := env("SERVITOR_ACTOR"); v != "" {
		a.Actor = v
	}
	return a
}

// NormHost is the short, lowercased host name: "Adirondack.local" and
// "adirondack" are one machine.
func NormHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	return h
}
