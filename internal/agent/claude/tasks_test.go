package claude

import (
	"testing"

	"github.com/seonl/agentview/internal/events"
)

func TestTaskCreationsAreCounted(t *testing.T) {
	list := newTaskList()

	for i := 1; i <= 3; i++ {
		done, total, ok := list.observe(toolTaskCreate, taskFields{})
		if !ok {
			t.Fatal("creation was not counted")
		}
		if done != 0 || total != i {
			t.Errorf("after %d creations: %d/%d", i, done, total)
		}
	}
}

func TestCompletionsAdvanceTheCount(t *testing.T) {
	list := newTaskList()
	list.observe(toolTaskUpdate, taskFields{TaskID: "a", Status: "pending"})
	list.observe(toolTaskUpdate, taskFields{TaskID: "b", Status: "pending"})

	done, total, _ := list.observe(toolTaskUpdate, taskFields{TaskID: "a", Status: "completed"})
	if done != 1 || total != 2 {
		t.Errorf("got %d/%d, want 1/2", done, total)
	}
}

// The same task completed twice must not count twice.
func TestRepeatedCompletionCountsOnce(t *testing.T) {
	list := newTaskList()
	list.observe(toolTaskUpdate, taskFields{TaskID: "a", Status: "completed"})

	done, total, _ := list.observe(toolTaskUpdate, taskFields{TaskID: "a", Status: "completed"})
	if done != 1 || total != 1 {
		t.Errorf("got %d/%d, want 1/1", done, total)
	}
}

// A task reopened after completion goes back to pending.
func TestReopenedTaskIsNoLongerDone(t *testing.T) {
	list := newTaskList()
	list.observe(toolTaskUpdate, taskFields{TaskID: "a", Status: "completed"})

	done, total, _ := list.observe(toolTaskUpdate, taskFields{TaskID: "a", Status: "in_progress"})
	if done != 0 || total != 1 {
		t.Errorf("got %d/%d, want 0/1", done, total)
	}
}

// A tool that rewrites the whole list replaces the count rather than adding
// to it, or every call would inflate the total.
func TestWholeListToolReplacesTheCount(t *testing.T) {
	list := newTaskList()

	todos := taskFields{}
	todos.Todos = append(todos.Todos, struct {
		Status string `json:"status"`
	}{Status: "completed"}, struct {
		Status string `json:"status"`
	}{Status: "pending"})

	for i := 0; i < 3; i++ {
		done, total, ok := list.observe(toolTodoWrite, todos)
		if !ok {
			t.Fatal("list was not read")
		}
		if done != 1 || total != 2 {
			t.Errorf("call %d: got %d/%d, want 1/2", i+1, done, total)
		}
	}
}

func TestStatusSpellings(t *testing.T) {
	for _, status := range []string{"completed", "Complete", "DONE", " done "} {
		if !isComplete(status) {
			t.Errorf("%q not recognized as complete", status)
		}
	}
	for _, status := range []string{"pending", "in_progress", "", "deleted"} {
		if isComplete(status) {
			t.Errorf("%q wrongly counted as complete", status)
		}
	}
}

// A tool that says nothing about the list must not produce a count.
func TestUnrelatedToolsReportNothing(t *testing.T) {
	list := newTaskList()

	for _, tool := range []string{"Read", "Bash", "WebFetch", ""} {
		if _, _, ok := list.observe(tool, taskFields{}); ok {
			t.Errorf("%q produced a count", tool)
		}
	}
}

// An update with no task id says nothing usable.
func TestUpdateWithoutAnIDIsIgnored(t *testing.T) {
	if _, _, ok := newTaskList().observe(toolTaskUpdate, taskFields{Status: "completed"}); ok {
		t.Error("an update with no id produced a count")
	}
}

// The stream must turn task bookkeeping into a progress event.
func TestStreamEmitsProgressFromTaskTools(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"assistant","message":{"content":[
	  {"type":"tool_use","id":"t1","name":"TaskCreate","input":{"subject":"first"}}
	]}}`

	got := tr.translateLine([]byte(line))
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if got[0].Type != events.TaskProgress {
		t.Fatalf("Type = %q, want %q", got[0].Type, events.TaskProgress)
	}
	if got[0].Done != 0 || got[0].Total != 1 {
		t.Errorf("got %d/%d, want 0/1", got[0].Done, got[0].Total)
	}
	if !got[0].Valid() {
		t.Error("progress event is invalid")
	}
}

// A turn's real cost has to reach the UI, since a long session re-sends its
// history every turn and that is what makes it expensive.
func TestStreamReportsTurnUsage(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"result","subtype":"success","total_cost_usd":0.42,
	  "usage":{"input_tokens":1200,"output_tokens":800,
	           "cache_read_input_tokens":40000,"cache_creation_input_tokens":2000}}`

	got := tr.translateLine([]byte(line))
	if len(got) != 2 {
		t.Fatalf("got %d events, want a status and a usage report: %+v", len(got), got)
	}

	session := got[1].Session
	if session == nil {
		t.Fatal("no session report")
	}
	// Cached input still counts towards the context the turn carried.
	if want := 1200 + 40000 + 2000; session.InputTokens != want {
		t.Errorf("InputTokens = %d, want %d", session.InputTokens, want)
	}
	if session.OutputTokens != 800 {
		t.Errorf("OutputTokens = %d, want 800", session.OutputTokens)
	}
	if session.CostUSD != 0.42 {
		t.Errorf("CostUSD = %v, want 0.42", session.CostUSD)
	}
	if session.Turns != 1 {
		t.Errorf("Turns = %d, want 1", session.Turns)
	}
}

// A turn that reports no usage must not fabricate one.
func TestStreamWithoutUsageReportsNothing(t *testing.T) {
	tr := newStreamTranslator("/proj")

	got := tr.translateLine([]byte(`{"type":"result","subtype":"success"}`))
	if len(got) != 1 {
		t.Fatalf("got %d events, want only the status change: %+v", len(got), got)
	}
}

// Turns accumulate, so the count reflects the whole session.
func TestStreamCountsTurns(t *testing.T) {
	tr := newStreamTranslator("/proj")
	line := []byte(`{"type":"result","usage":{"input_tokens":10}}`)

	var last int
	for i := 1; i <= 3; i++ {
		got := tr.translateLine(line)
		last = got[len(got)-1].Session.Turns
	}
	if last != 3 {
		t.Errorf("Turns = %d after three turns, want 3", last)
	}
}

func TestStreamEmitsProgressFromWholeListTool(t *testing.T) {
	tr := newStreamTranslator("/proj")

	line := `{"type":"assistant","message":{"content":[
	  {"type":"tool_use","id":"t1","name":"TodoWrite","input":{"todos":[
	    {"status":"completed"},{"status":"completed"},{"status":"pending"}
	  ]}}
	]}}`

	got := tr.translateLine([]byte(line))
	if len(got) != 1 || got[0].Type != events.TaskProgress {
		t.Fatalf("got %+v, want one progress event", got)
	}
	if got[0].Done != 2 || got[0].Total != 3 {
		t.Errorf("got %d/%d, want 2/3", got[0].Done, got[0].Total)
	}
}
