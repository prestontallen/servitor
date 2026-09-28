package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// toneInstalled reports whether install.sh linked the servitor-tone skill
// into any agent skill root under home (Claude, Hermes). install.sh links
// every detected root together, so one is as good as all.
func toneInstalled(home string) bool {
	for _, root := range []string{".claude/skills", ".hermes/skills"} {
		if _, err := os.Stat(filepath.Join(home, root, "servitor-tone")); err == nil {
			return true
		}
	}
	return false
}

// handoffFields are the ticket fields that make up the handoff record, in
// print order. Agents set them with `servitor set` in the same command as
// the transition they describe (skills/servitor: Handoff).
var handoffFields = []string{"branch", "worktree", "head", "pushed", "staging", "checkpoint", "next"}

const defaultBranchTemplate = "agent/<agentname>/<ticket-slug>"

var (
	placeholderRE = regexp.MustCompile(`<([a-z][a-z0-9-]*)>`)
	jiraKeyRE     = regexp.MustCompile(`^[A-Z][A-Z0-9]+-[0-9]+$`)
)

// jiraKey reads the ticket's jira field, a /browse/ link or a bare key,
// and returns the key. A board or search link fails the PROJ-123 shape.
func jiraKey(v any) (string, bool) {
	s, _ := v.(string)
	s, _, _ = strings.Cut(s, "?")
	s, _, _ = strings.Cut(s, "#")
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	s = s[strings.LastIndex(s, "/")+1:]
	return s, jiraKeyRE.MatchString(s)
}

// branchName fills tmpl for the focused ticket. <agentname> and
// <ticket-slug> always resolve; any other <name> reads the ticket field of
// that name (<jira> through jiraKey). With no focused ticket those stay
// placeholders. missing lists the fields that did not resolve.
func branchName(tmpl, agent, slug string, fields map[string]any, focused bool) (name string, missing []string) {
	name = placeholderRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		switch k := m[1 : len(m)-1]; {
		case k == "agentname":
			return agent
		case k == "ticket-slug":
			return slug
		case !focused:
			return m
		case k == "jira":
			if key, ok := jiraKey(fields[k]); ok {
				return key
			}
		default:
			if s, ok := fields[k].(string); ok && s != "" {
				return s
			}
		}
		missing = append(missing, m[1:len(m)-1])
		return m
	})
	return name, missing
}

// preflight prints the where-am-I header for the SessionStart hook: cwd,
// canonical checkout vs linked worktree, branch, distance behind
// origin/main, and who holds the focused card. Best effort: every git call
// is capped, a failure prints what it could, and nothing here can block a
// session. doc is the ctx aggregate for the focused ticket, or nil.
func preflight(w io.Writer, dir, actor, agentHint string, doc []byte) {
	git := func(timeout time.Duration, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}

	// the tone skill is opt-in at install and mandatory once linked; the
	// hook is in context every session, so it is where that gets said
	if home, err := os.UserHomeDir(); err == nil && toneInstalled(home) {
		fmt.Fprintln(w, "register: servitor-tone ON")
	}
	fmt.Fprintf(w, "cwd: %s\n", dir)
	top, err := git(time.Second, "rev-parse", "--show-toplevel")
	if err != nil {
		fmt.Fprintln(w, "not a git checkout")
		return
	}
	// first "worktree <path>" line of the porcelain list is the main checkout
	canonical := false
	if wt, err := git(time.Second, "worktree", "list", "--porcelain"); err == nil {
		first, _, _ := strings.Cut(wt, "\n")
		canonical = strings.TrimPrefix(first, "worktree ") == top
	}
	// separate call: --abbrev-ref HEAD fails on an unborn branch, and that
	// must not read as "not a git checkout"
	branch, _ := git(time.Second, "rev-parse", "--abbrev-ref", "HEAD")
	// every agent on the host shares the canonical checkout, so a ticket
	// branch checked out there breaks everyone's "where am I"; "" and
	// "HEAD" are unborn/detached and get the plain line.
	offMain := canonical && branch != "" && branch != "HEAD" && branch != "main"
	switch {
	case offMain:
		fmt.Fprintf(w, "canonical checkout on %s: MUST BE ON MAIN. Someone checked a branch out here. Restore before anything else:\n", branch)
		// a WIP commit, not a stash: the stash stack is shared by every
		// worktree and any session may pop it
		fmt.Fprintf(w, "  git -C %s commit -qam wip; git -C %s checkout main; git worktree add ../%s-worktrees/<its-slug> %s\n",
			top, top, filepath.Base(top), branch)
	case canonical:
		fmt.Fprintf(w, "canonical checkout on %s: CREATE A WORKTREE BEFORE EDITING\n", branch)
	default:
		fmt.Fprintf(w, "worktree %s on %s\n", top, branch)
	}

	_, fetchErr := git(2*time.Second, "fetch", "-q", "origin", "main")
	n, err := git(time.Second, "rev-list", "--count", "HEAD..origin/main")
	switch {
	case err != nil:
		fmt.Fprintln(w, "origin/main: unavailable")
	case fetchErr != nil:
		fmt.Fprintf(w, "behind origin/main by %s (origin unreachable, stale)\n", n)
	default:
		fmt.Fprintf(w, "behind origin/main by %s\n", n)
	}

	slug := "<ticket-slug>"
	var fields map[string]any
	focused := false
	if doc != nil {
		var f struct {
			Slug        string         `json:"slug"`
			Status      string         `json:"status"`
			ActiveBy    string         `json:"active_by"`
			ActiveSince string         `json:"active_since"`
			Fields      map[string]any `json:"fields"`
		}
		if json.Unmarshal(doc, &f) == nil && f.Slug != "" {
			slug, fields, focused = f.Slug, f.Fields, true
			line := fmt.Sprintf("card: %s %s", f.Slug, f.Status)
			if f.ActiveBy != "" && f.ActiveBy != actor {
				line += fmt.Sprintf(", active by %s since %s: do not start it", f.ActiveBy, f.ActiveSince)
			}
			fmt.Fprintln(w, line)
			// the handoff record: where the work is and what may happen
			// next, so a cold agent reads it before anything else
			for _, k := range handoffFields {
				if v, ok := f.Fields[k]; ok && v != nil && v != "" {
					fmt.Fprintf(w, "handoff %s: %v\n", k, v)
				}
			}
		}
	}
	// servitor.branchTemplate is the operator's naming preference; git's
	// own local-over-global precedence decides which one applies
	tmpl, _ := git(time.Second, "config", "--get", "servitor.branchTemplate")
	if tmpl != "" && !strings.Contains(tmpl, "<ticket-slug>") {
		fmt.Fprintf(w, "servitor.branchTemplate %q has no <ticket-slug>: ignored, using the default\n", tmpl)
		tmpl = ""
	}
	if tmpl == "" {
		tmpl = defaultBranchTemplate
	}
	agent := strings.TrimPrefix(actor, "agent:")
	name, missing := branchName(tmpl, agent, slug, fields, focused)
	// Jira mode: a branch without the key never links, and renaming a
	// pushed branch breaks the link Jira already made, so no key, no branch
	noJira := slices.Contains(missing, "jira")
	switch {
	case noJira:
		if v, ok := fields["jira"]; ok && v != nil && v != "" {
			fmt.Fprintf(w, "jira field \"%v\" is not a Jira key (want PROJ-123): fix it before the branch\n", v)
		} else {
			fmt.Fprintf(w, "no jira key on %s: ask the human for the Jira issue link before the branch\n", slug)
		}
		fmt.Fprintf(w, "  servitor set %s jira=<jira-issue-link>\n", slug)
	case len(missing) > 0:
		fmt.Fprintf(w, "servitor.branchTemplate needs field %q, which %s lacks: using the default\n", missing[0], slug)
		name, _ = branchName(defaultBranchTemplate, agent, slug, fields, focused)
	case focused && strings.Contains(tmpl, "<jira>"):
		key, _ := jiraKey(fields["jira"])
		fmt.Fprintf(w, "jira: %s: commit subjects and the PR title start with it\n", key)
	}
	if canonical && !noJira {
		fmt.Fprintf(w, "  git fetch origin\n  git worktree add -b %s ../%s-worktrees/%s origin/main\n",
			name, filepath.Base(top), slug)
		// cd moves a Hermes session (the tool call runs where it lands);
		// Claude Code needs the EnterWorktree tool. Claude is the default
		// wording for any agent the decision did not name.
		switch agentName(actor, agentHint) {
		case "hermes":
			fmt.Fprintf(w, "  Hermes: then cd ../%s-worktrees/%s (cd alone moves the session)\n",
				filepath.Base(top), slug)
		default:
			fmt.Fprintf(w, "  Claude Code: then EnterWorktree path=../%s-worktrees/%s (cd alone leaves the session here)\n",
				filepath.Base(top), slug)
		}
	}
}
