package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/git"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// withGit returns a sized model whose project has the given Git status.
func withGit(t *testing.T, status git.Status) Model {
	t.Helper()

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Project.Git = status

	m, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return m.(Model)
}

func treeText(m Model) string {
	return ansi.Strip(strings.Join(m.treePanel(computeLayout(100, 40), time.Now()), "\n"))
}

func TestModifiedAndUntrackedFilesAreMarked(t *testing.T) {
	m := withGit(t, git.Status{
		Branch: "main",
		Files: map[string]git.FileStatus{
			"Assets/Scripts/Puzzle/Valve.cs":    git.Modified,
			"Assets/Scripts/Rooms/WaterRoom.cs": git.Untracked,
		},
	})

	tree := treeText(m)
	if line := lineContaining(tree, "Valve.cs"); !strings.Contains(line, "M") {
		t.Errorf("modified file not marked: %q", line)
	}
	if line := lineContaining(tree, "WaterRoom.cs"); !strings.Contains(line, "+") {
		t.Errorf("untracked file not marked: %q", line)
	}
}

func TestUnchangedFilesAreNotMarked(t *testing.T) {
	m := withGit(t, git.Status{Branch: "main"})

	line := lineContaining(treeText(m), "Valve.cs")
	if strings.ContainsAny(line, "M+") {
		t.Errorf("clean file carries a Git marker: %q", line)
	}
}

// The gutter is shared, and what the agent is doing right now outranks the
// fact that a file differs from the last commit.
func TestActivityOutranksGitInTheGutter(t *testing.T) {
	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Project.Git = git.Status{
		Branch: "main",
		Files:  map[string]git.FileStatus{"Assets/Scripts/Puzzle/Valve.cs": git.Modified},
	}

	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ := send(model.(Model), edit("Assets/Scripts/Puzzle/Valve.cs"))

	line := lineContaining(treeText(m), "Valve.cs")
	if !strings.Contains(line, "●") {
		t.Errorf("active file lost its activity marker to Git: %q", line)
	}
	if strings.Contains(line, "M") {
		t.Errorf("both markers shown at once: %q", line)
	}
}

// Directories are not tracked by Git; only their contents are.
func TestDirectoriesCarryNoGitMarker(t *testing.T) {
	m := withGit(t, git.Status{
		Branch: "main",
		Files:  map[string]git.FileStatus{"Assets/Scripts": git.Modified},
	})

	line := lineContaining(treeText(m), "Scripts/")
	if strings.ContainsAny(line, "M+") {
		t.Errorf("directory carries a Git marker: %q", line)
	}
}

func TestBranchIsShown(t *testing.T) {
	m := withGit(t, git.Status{Branch: "feature/live-state"})

	if out := ansi.Strip(m.View()); !strings.Contains(out, "feature/live-state") {
		t.Errorf("branch not shown:\n%s", out)
	}
}

// A dirty tree is flagged, but without counts or anything to act on.
func TestDirtyTreeIsFlagged(t *testing.T) {
	clean := withGit(t, git.Status{Branch: "main"})
	if got := clean.branchLabel(); got != "main" {
		t.Errorf("clean label = %q, want main", got)
	}

	dirty := withGit(t, git.Status{
		Branch: "main",
		Files:  map[string]git.FileStatus{"a.cs": git.Modified},
	})
	if got := dirty.branchLabel(); got != "main*" {
		t.Errorf("dirty label = %q, want main*", got)
	}
}

func TestDetachedHeadIsLabelled(t *testing.T) {
	m := withGit(t, git.Status{Detached: true})

	if got := m.branchLabel(); got != "detached" {
		t.Errorf("label = %q, want detached", got)
	}
}

// Outside a repository the UI shows no Git information at all, rather than an
// empty slot where a branch would be.
func TestNoRepositoryShowsNoGitInformation(t *testing.T) {
	m := withGit(t, git.Status{})

	if got := m.branchLabel(); got != "" {
		t.Errorf("label = %q, want empty outside a repository", got)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "q quit") {
		t.Errorf("bottom bar broke without Git:\n%s", out)
	}
}

// Git information must not push the layout out of shape at any width.
func TestGitLabelKeepsTheLayoutIntact(t *testing.T) {
	status := git.Status{
		Branch: "a-very-long-branch-name-someone-actually-used",
		Files:  map[string]git.FileStatus{"a.cs": git.Modified},
	}

	for _, w := range []int{40, 60, 80, 120, 200} {
		st := state.New("/proj")
		st.Project.Tree = project.MockTree()
		st.Project.Git = status

		model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for i, line := range strings.Split(model.(Model).View(), "\n") {
			if got := ansi.StringWidth(line); got != w {
				t.Errorf("width %d line %d: got %d", w, i, got)
			}
		}
	}
}

// A repository snapshot arriving must schedule the next read, or Git state
// would freeze after the first look.
func TestGitRefreshReschedulesItself(t *testing.T) {
	m := withGit(t, git.Status{})

	next, cmd := m.Update(gitMsg(git.Status{Branch: "main"}))
	if cmd == nil {
		t.Fatal("git refresh did not reschedule")
	}
	if got := next.(Model).st.Project.Git.Branch; got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
}
