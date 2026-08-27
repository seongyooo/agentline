package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/seongyooo/agentline/internal/events"
	"github.com/seongyooo/agentline/internal/git"
	"github.com/seongyooo/agentline/internal/project"
	"github.com/seongyooo/agentline/internal/state"
)

// TestLook renders a busy frame with the escape codes left in, so the styling
// can be judged rather than guessed at. Set LOOK=1.
//
// Colour is checked by eye here on purpose: a test can assert that a style was
// applied, but not that the result reads well.
func TestLook(t *testing.T) {
	if os.Getenv("LOOK") == "" {
		t.Skip("set LOOK=1")
	}

	st := state.New("/proj")
	st.Project.Tree = project.MockTree()
	st.Project.Git = git.Status{
		Branch: "main",
		Files: map[string]git.FileStatus{
			"Assets/Scripts/Puzzle/Valve.cs":    git.Modified,
			"Assets/Scripts/Rooms/WaterRoom.cs": git.Untracked,
		},
	}

	now := time.Now()
	// A real session is not eight actions in five minutes. The strip is drawn
	// from counts, so judging it on sparse data judges the wrong picture.
	seedBusySession(st, now.Add(-25*time.Minute), now.Add(-6*time.Minute))

	for _, e := range []events.Event{
		{Type: events.UserPrompt, Message: "Wire the drainage system to the valve", Timestamp: now.Add(-5 * time.Minute), Source: "claude-code"},
		{Type: events.TaskProgress, Done: 2, Total: 5, Timestamp: now.Add(-5 * time.Minute), Source: "claude-code",
			Tasks: []events.Task{
				{Text: "Read the drainage system", Done: true},
				{Text: "Add the valve interface", Done: true},
				{Text: "Wire the valve to the pump", Doing: "Wiring the valve to the pump", Now: true},
				{Text: "Update the room script"},
				{Text: "Run the tests"},
			}},
		{Type: events.FileRead, Path: "Assets/Scripts/Rooms/WaterRoom.cs", Timestamp: now.Add(-4 * time.Minute), Source: "claude-code"},
		{Type: events.CommandStart, Command: "go test ./internal/git", Message: "Run the git package tests", Timestamp: now.Add(-3 * time.Minute), Source: "claude-code"},
		{Type: events.CommandEnd, Command: "go test ./internal/git", Message: "Run the git package tests", Timestamp: now.Add(-2 * time.Minute), Source: "claude-code"},
		{Type: events.FileEdit, Path: "Assets/Scripts/Puzzle/Valve.cs", Timestamp: now.Add(-90 * time.Second), Source: "claude-code"},
		{Type: events.AgentError, Message: "go vet found an unused variable", Timestamp: now.Add(-60 * time.Second), Source: "claude-code"},
		{Type: events.FileEdit, Path: "Assets/Scripts/Puzzle/DrainSystem.cs", Timestamp: now.Add(-4 * time.Second), Source: "claude-code"},
		{Type: events.PermissionAsk, Timestamp: now, Source: "claude-code", Ask: &events.Ask{
			ID: "req-1", Tool: "Write", Title: "Write", Target: "DrainSystem.cs", Mode: "acceptEdits",
			Reason: "Claude requested permissions to edit Assets/Scripts/Puzzle/DrainSystem.cs which is a sensitive file.",
		}},
		{Type: events.SessionInfo, Timestamp: now, Source: "claude-code", Session: &events.Session{
			Model:         "claude-opus-5-20260514",
			ContextWindow: 200_000, ContextPercent: 0.31,
			Capabilities: events.Capabilities{PermissionMode: "acceptEdits"},
			Limits: map[string]events.Limit{
				"five_hour": {Used: 0.62, ResetsAt: now.Add(2 * time.Hour)},
				"seven_day": {Used: 0.28, ResetsAt: now.Add(90 * time.Hour)},
			},
		}},
	} {
		st.Apply(e)
	}

	// Two sizes, because the styling has to survive the panels collapsing as
	// well as look right when everything fits.
	for _, size := range []struct{ w, h int }{{100, 30}, {76, 22}} {
		model, _ := New(st, nil, nil).WithSender(&fakeSender{}).Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})

		fmt.Println(strings.Repeat("=", size.w-8), size.w, "x", size.h)
		fmt.Println(model.(Model).View())
		fmt.Println(strings.Repeat("=", size.w))
	}
}

// seedBusySession fills the pulse with the shape of an ordinary working
// stretch: bursts of reading and editing, a run of tests, a failure, and the
// quiet gaps in between.
func seedBusySession(st *state.State, from, to time.Time) {
	kinds := []events.Type{events.FileRead, events.FileRead, events.FileEdit, events.CommandStart, events.FileRead}
	commands := []string{
		"dotnet build",
		"dotnet test --filter Puzzle",
		"git status --porcelain",
	}
	paths := []string{
		"Assets/Scripts/Puzzle/Valve.cs",
		"Assets/Scripts/Rooms/WaterRoom.cs",
		"Assets/Scripts/Core/Boot.cs",
		"Assets/Scripts/Player/Move.cs",
	}

	span := to.Sub(from)
	for i := 0; i < 220; i++ {
		at := from.Add(time.Duration(float64(span) * float64(i) / 220))

		// Two quiet stretches, so the row has gaps to make its bursts mean
		// something.
		if share := float64(i) / 220; (share > 0.22 && share < 0.34) || (share > 0.63 && share < 0.71) {
			continue
		}

		e := events.Event{Type: kinds[i%len(kinds)], Timestamp: at, Source: "claude-code"}
		switch e.Type {
		case events.CommandStart:
			e.Command = commands[i%len(commands)]
		default:
			e.Path = paths[i%len(paths)]
		}
		if i == 96 {
			e = events.Event{Type: events.AgentError, Message: "build failed", Timestamp: at, Source: "claude-code"}
		}
		st.Apply(e)
	}
}
