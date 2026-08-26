// Package git reads just enough repository state to sharpen the picture of
// where work is happening: the current branch, and which files have changed.
//
// It is deliberately not a Git integration. There is no staging, no diffing,
// no history, and no way to change anything — lazygit already does that well.
// If something here cannot be answered by "does this help you see what the
// agent is doing", it does not belong in this package.
package git

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileStatus is how a file differs from the last commit. The distinction
// between staged and unstaged is deliberately not made: acting on it would
// require a Git UI, which AgentView is not.
type FileStatus int

const (
	Unchanged FileStatus = iota
	Modified
	Untracked
)

// Status is a snapshot of the repository.
//
// The zero value is what a directory that is not a repository looks like, so
// callers can render it without special-casing.
type Status struct {
	// Branch is empty outside a repository, and outside one that has commits.
	Branch string

	// Detached marks a checkout that is not on a branch.
	Detached bool

	// Files maps a root-relative slash path to how it changed. Only changed
	// files appear.
	Files map[string]FileStatus
}

// Dirty reports whether the working tree has any changes.
func (s Status) Dirty() bool { return len(s.Files) > 0 }

// Of returns a file's status, or Unchanged if it has none.
func (s Status) Of(path string) FileStatus {
	if s.Files == nil {
		return Unchanged
	}
	return s.Files[path]
}

// Available reports whether the directory turned out to be a repository.
func (s Status) Available() bool { return s.Branch != "" || s.Detached || s.Files != nil }

// Load reads the repository containing root.
//
// A directory that is not a repository, or a machine with no git installed, is
// not an error: it returns the zero Status, and AgentView simply shows no Git
// information rather than failing.
func Load(ctx context.Context, root string) Status {
	top, err := run(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return Status{}
	}
	topLevel := strings.TrimSpace(top)

	// --untracked-files=all lists files inside a new directory individually.
	// The default collapses them to one entry with a trailing slash, which
	// would leave every file in a newly created package unmarked — exactly
	// the case where seeing what is new matters most.
	out, err := run(ctx, root, "status", "--porcelain", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return Status{}
	}
	return parse(out, topLevel, root)
}

// run executes a git command in root and returns its stdout.
func run(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// parse reads porcelain v1 output. Records are NUL-separated, which keeps
// paths containing spaces or newlines intact.
func parse(out, topLevel, root string) Status {
	status := Status{Files: map[string]FileStatus{}}

	records := strings.Split(out, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}

		if strings.HasPrefix(record, "## ") {
			status.Branch, status.Detached = parseBranch(record[3:])
			continue
		}
		if len(record) < 4 {
			continue // not a status entry
		}

		code, path := record[:2], record[3:]
		if code[0] == 'R' || code[0] == 'C' {
			// A rename or copy is followed by its source path, which is of no
			// interest: only where the file is now matters.
			i++
		}

		rel := relative(topLevel, root, path)
		if rel == "" {
			continue // outside the directory AgentView is showing
		}
		if code == "??" {
			status.Files[rel] = Untracked
		} else {
			status.Files[rel] = Modified
		}
	}
	return status
}

// parseBranch reads the porcelain branch header.
func parseBranch(header string) (branch string, detached bool) {
	if header == "HEAD (no branch)" {
		return "", true
	}
	// A repository with no commits yet reports the branch it would create.
	if rest, ok := strings.CutPrefix(header, "No commits yet on "); ok {
		header = rest
	}
	// Trim the upstream and any ahead/behind counts.
	if i := strings.Index(header, "..."); i >= 0 {
		header = header[:i]
	}
	if i := strings.Index(header, " "); i >= 0 {
		header = header[:i]
	}
	return header, false
}

// relative converts a repository-relative path to one relative to the
// directory AgentView is showing, which is not always the repository root.
// Anything outside it returns empty so it is dropped.
func relative(topLevel, root, repoPath string) string {
	// Git marks directory entries with a trailing slash; the tree keys on the
	// path without one.
	repoPath = strings.TrimSuffix(repoPath, "/")

	abs := filepath.Join(topLevel, filepath.FromSlash(repoPath))

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return rel
}
