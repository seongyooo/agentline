package tui

import (
	"fmt"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// TestReadmeFrame renders the frame the README shows, so the picture in the
// documentation is one the program actually produces. Set README_FRAME=1.
func TestReadmeFrame(t *testing.T) {
	if os.Getenv("README_FRAME") == "" {
		t.Skip("set README_FRAME=1")
	}

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()

	now := time.Now()
	for _, e := range []events.Event{
		{Type: events.UserPrompt, Message: "Add Git awareness to the header", Timestamp: now.Add(-4 * time.Minute), Source: "claude"},
		{Type: events.TaskProgress, Done: 2, Total: 4, Timestamp: now.Add(-4 * time.Minute), Source: "claude"},
		{Type: events.FileRead, Path: "Assets/Scripts/Rooms/WaterRoom.cs", Timestamp: now.Add(-3 * time.Minute), Source: "claude"},
		{Type: events.CommandStart, Command: "go test ./internal/git", Message: "Run the git package tests", Timestamp: now.Add(-2 * time.Minute), Source: "claude"},
		{Type: events.CommandEnd, Command: "go test ./internal/git", Message: "Run the git package tests", Timestamp: now.Add(-time.Minute), Source: "claude"},
		{Type: events.FileEdit, Path: "Assets/Scripts/Puzzle/DrainSystem.cs", Timestamp: now.Add(-3 * time.Second), Source: "claude"},
		{Type: events.SessionInfo, Timestamp: now, Source: "claude", Session: &events.Session{
			Model:         "claude-opus-5-20260514",
			ContextWindow: 200_000, ContextPercent: 0.31,
			Capabilities: events.Capabilities{PermissionMode: "default"},
			Limits: map[string]events.Limit{
				"five_hour": {Used: 0.62, ResetsAt: time.Date(2026, 8, 27, 19, 40, 0, 0, time.Local)},
				"seven_day": {Used: 0.28, ResetsAt: time.Date(2026, 8, 31, 13, 0, 0, 0, time.Local)},
			},
		}},
	} {
		st.Apply(e)
	}

	const width, height = 100, 28
	model, _ := New(st, nil, nil).Update(tea.WindowSizeMsg{Width: width, Height: height})

	fmt.Printf("rendered at %dx%d\n%s\n", width, height, ansi.Strip(model.(Model).View()))
}
