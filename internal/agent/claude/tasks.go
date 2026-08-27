package claude

import (
	"strings"

	"github.com/seongyooo/agentline/internal/events"
)

// Tools by which the agent keeps a task list. Claude Code has used more than
// one shape for this, so both are read: a per-task tool that creates and
// updates entries, and a whole-list tool that rewrites the list each call.
const (
	toolTaskCreate = "TaskCreate"
	toolTaskUpdate = "TaskUpdate"
	toolTodoWrite  = "TodoWrite"
)

// taskFields are the parts of a task tool's arguments that say what the list
// looks like. Anything else the tool carries is ignored.
type taskFields struct {
	TaskID string `json:"taskId"`
	Status string `json:"status"`

	// Description is what a per-task tool calls the task. The whole-list tool
	// uses its own fields, below.
	Description string `json:"description"`

	// Todos is the whole list, when the tool rewrites it in one call.
	//
	// Content is the imperative wording and ActiveForm the present-tense one;
	// the agent is asked for both so a UI can say either. Reading only Status
	// here, which is how this started, threw the words away and left a bar
	// with no idea what it was counting.
	Todos []todoEntry `json:"todos"`
}

// todoEntry is one item of the whole-list tool's argument.
type todoEntry struct {
	Content    string `json:"content"`
	ActiveForm string `json:"activeForm"`
	Status     string `json:"status"`
}

// taskList tracks the agent's task list as its tool calls go by.
//
// It counts only what the agent itself reported. If the agent keeps no list,
// the count stays unknown and AgentLine shows no progress, rather than
// inventing a number for work whose real extent nobody can see.
type taskList struct {
	// done records completion by task id, so a task completed twice is not
	// counted twice and a reopened one goes back to pending.
	done map[string]bool

	// created preserves the order tasks were first seen, which is also the
	// count of tasks the agent has planned.
	created []string

	// entry holds what each task says, by the same ids.
	entry map[string]events.Task
}

func newTaskList() *taskList {
	return &taskList{done: map[string]bool{}, entry: map[string]events.Task{}}
}

// observe folds one task tool call into the list and reports the resulting
// counts. ok is false when the call said nothing about the list.
func (t *taskList) observe(tool string, fields taskFields) (done, total int, ok bool) {
	switch tool {
	case toolTodoWrite:
		// The whole list arrives at once and replaces what was there.
		if fields.Todos == nil {
			return 0, 0, false
		}
		t.reset()
		for i, todo := range fields.Todos {
			id := string(rune('a' + i%26))
			t.add(id)
			t.done[id] = isComplete(todo.Status)
			t.entry[id] = events.Task{
				Text:  todo.Content,
				Doing: todo.ActiveForm,
				Done:  isComplete(todo.Status),
				Now:   isActive(todo.Status),
			}
		}

	case toolTaskCreate:
		id := newTaskID(len(t.created))
		t.add(id)
		t.describe(id, fields.Description, fields.Status)

	case toolTaskUpdate:
		if fields.TaskID == "" {
			return 0, 0, false
		}
		t.add(fields.TaskID)
		t.done[fields.TaskID] = isComplete(fields.Status)
		t.describe(fields.TaskID, fields.Description, fields.Status)

	default:
		return 0, 0, false
	}

	return t.counts()
}

// describe records what a per-task tool said, keeping wording it gave before
// when this call did not repeat it.
func (t *taskList) describe(id, text, status string) {
	entry := t.entry[id]
	if text != "" {
		entry.Text = text
	}
	entry.Done = isComplete(status)
	entry.Now = isActive(status)
	t.entry[id] = entry
}

// Tasks is the list as the agent last described it, in the order it planned.
func (t *taskList) Tasks() []events.Task {
	tasks := make([]events.Task, 0, len(t.created))
	for _, id := range t.created {
		entry := t.entry[id]
		entry.Done = t.done[id]
		tasks = append(tasks, entry)
	}
	return tasks
}

func (t *taskList) reset() {
	t.done = map[string]bool{}
	t.entry = map[string]events.Task{}
	t.created = nil
}

// add records a task id the first time it is seen.
func (t *taskList) add(id string) {
	if _, seen := t.done[id]; seen {
		return
	}
	t.done[id] = false
	t.created = append(t.created, id)
}

func (t *taskList) counts() (done, total int, ok bool) {
	for _, id := range t.created {
		if t.done[id] {
			done++
		}
	}
	return done, len(t.created), len(t.created) > 0
}

// newTaskID names a task the agent created without giving an id, so that
// creations are still counted.
func newTaskID(n int) string {
	return "created-" + string(rune('0'+n%10)) + string(rune('a'+n/10%26))
}

// isActive reads the status that marks the one task being worked on now.
func isActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in_progress", "in-progress", "active", "running":
		return true
	}
	return false
}

// isComplete reads the several spellings a status has been reported with.
func isComplete(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done":
		return true
	}
	return false
}
