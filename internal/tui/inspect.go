package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/seongyooo/agentline/internal/git"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// inspectPanel describes the selected file or directory in place of the
// mission column.
//
// Everything here was observed: what is on disk, what Git says, and what the
// agent did with it. AgentLine does not describe what a file is *for* — that
// would mean reading it and summarizing, which is a judgement it has no way
// to make honestly and is not what this tool is.
func (m Model) inspectPanel(l Layout, node *project.Node, now time.Time) []string {
	width := l.MissionWidth()

	kind := "FILE"
	if node.Dir {
		kind = "DIRECTORY"
	}
	lines := []string{
		m.panelHeading(kind, focusInspect, 0, 1, 0, width),
		styleValue.Render(fitLine(node.Name, width)),
	}
	if node.Path != node.Name {
		lines = append(lines, styleDim.Render(fitLine(node.Path, width)))
	}
	lines = append(lines, "")

	lines = append(lines, m.inspectFacts(node, now)...)
	lines = append(lines, "")
	lines = append(lines, m.inspectHistory(node, width)...)

	return append(lines, "", styleDim.Render("esc back"))
}

// inspectFacts lists what can be read about the entry without opening it.
func (m Model) inspectFacts(node *project.Node, now time.Time) []string {
	var lines []string

	info, err := os.Stat(filepath.Join(m.st.Project.Root, filepath.FromSlash(node.Path)))
	switch {
	case err != nil:
		// Announced but not written yet, or removed since the scan.
		if m.st.Pending(node.Path) {
			lines = append(lines, styleDim.Render("not written yet"))
		} else {
			lines = append(lines, styleDim.Render("not on disk"))
		}

	case node.Dir:
		lines = append(lines, styleDim.Render(fmt.Sprintf("%s entries", countChildren(node))))
		lines = append(lines, styleDim.Render("changed "+ago(info.ModTime(), now)))

	default:
		lines = append(lines, styleDim.Render(fmt.Sprintf("%s   changed %s",
			byteSize(info.Size()), ago(info.ModTime(), now))))
	}

	if status := m.gitLabel(node); status != "" {
		lines = append(lines, status)
	}
	return lines
}

// gitLabel says how the entry differs from the last commit, if it does.
func (m Model) gitLabel(node *project.Node) string {
	if node.Dir {
		return "" // Git tracks files, not directories
	}
	switch m.st.Project.Git.Of(node.Path) {
	case git.Modified:
		return styleWarn.Render("modified since the last commit")
	case git.Untracked:
		return styleDim.Render("not tracked by Git")
	}
	return ""
}

// inspectHistory is what the agent did with this entry during the session.
func (m Model) inspectHistory(node *project.Node, width int) []string {
	lines := []string{styleLabel.Render("AGENT ACTIVITY")}

	var seen int
	for i := len(m.st.Agent.Activity) - 1; i >= 0 && seen < inspectHistoryRows; i-- {
		a := m.st.Agent.Activity[i]
		if !touches(a, node) {
			continue
		}
		seen++
		lines = append(lines, styleDim.Render(fitLine(
			fmt.Sprintf("%s  %s", a.At.Format("15:04"), actionVerb(a.Kind)), width)))
	}

	if seen == 0 {
		lines = append(lines, styleDim.Render("none this session"))
	}
	return lines
}

// inspectHistoryRows bounds the history shown, which is a summary and not a log.
const inspectHistoryRows = 6

// touches reports whether an action was about this entry, counting anything
// inside a directory as touching the directory.
func touches(a state.Action, node *project.Node) bool {
	if !targetsAPath(a.Kind) {
		return false
	}
	if a.Target == node.Path {
		return true
	}
	return node.Dir && strings.HasPrefix(a.Target, node.Path+"/")
}

// countChildren describes how much a directory holds, saying when it has not
// been read rather than reporting zero.
func countChildren(node *project.Node) string {
	if !node.Loaded {
		return "not read"
	}
	return fmt.Sprint(len(node.Children))
}

// byteSize renders a file size in the units people read them in.
func byteSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// ago renders how long ago something happened, in the coarsest unit that
// still says something useful.
func ago(at, now time.Time) string {
	switch d := now.Sub(at); {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return at.Format("2006-01-02")
}
