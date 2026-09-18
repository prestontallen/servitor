package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// preflight prints the where-am-I header for the SessionStart hook: cwd,
// canonical checkout vs linked worktree, branch, distance behind
// origin/main, and who holds the focused card. Best effort: every git call
// is capped, a failure prints what it could, and nothing here can block a
// session. doc is the ctx aggregate for the focused ticket, or nil.
func preflight(w io.Writer, dir, actor string, doc []byte) {
	git := func(timeout time.Duration, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
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
	if canonical {
		fmt.Fprintf(w, "canonical checkout on %s: CREATE A WORKTREE BEFORE EDITING\n", branch)
	} else {
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
	if doc != nil {
		var f struct {
			Slug        string `json:"slug"`
			Status      string `json:"status"`
			ActiveBy    string `json:"active_by"`
			ActiveSince string `json:"active_since"`
		}
		if json.Unmarshal(doc, &f) == nil && f.Slug != "" {
			slug = f.Slug
			line := fmt.Sprintf("card: %s %s", f.Slug, f.Status)
			if f.ActiveBy != "" && f.ActiveBy != actor {
				line += fmt.Sprintf(", active by %s since %s: do not start it", f.ActiveBy, f.ActiveSince)
			}
			fmt.Fprintln(w, line)
		}
	}
	if canonical {
		name := strings.TrimPrefix(actor, "agent:")
		fmt.Fprintf(w, "  git fetch origin\n  git worktree add -b agent/%s/%s ../%s-worktrees/%s origin/main\n",
			name, slug, filepath.Base(top), slug)
	}
}
