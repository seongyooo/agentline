package tui

import "strings"

// Slash commands are the agent's, not AgentView's. The text is sent through
// untouched; all that happens here is completion from the list the session
// announced when it started, so nothing is ever offered that the agent did
// not say it accepts.

// slashPrefix is the word being completed, and whether one is being typed.
//
// Completion applies only to a command at the very start of the prompt. A
// slash later in a sentence is part of a path or a URL, and offering to
// complete it would get in the way of ordinary typing.
func (m Model) slashPrefix() (string, bool) {
	value := m.input.Value()
	if !strings.HasPrefix(value, "/") {
		return "", false
	}
	if strings.ContainsAny(value, " \t") {
		return "", false // the command is already typed; this is its argument
	}
	return value[1:], true
}

// slashMatches are the commands that start with what has been typed.
func (m Model) slashMatches() []string {
	prefix, ok := m.slashPrefix()
	if !ok {
		return nil
	}

	var matches []string
	for _, command := range m.st.SlashCommands() {
		if strings.HasPrefix(command, prefix) {
			matches = append(matches, command)
		}
	}
	return matches
}

// completeSlash fills in as much of the command as the matches agree on.
//
// Completing only the shared part means a key press never chooses between
// candidates on the user's behalf; it stops where the choice begins.
func (m *Model) completeSlash() bool {
	matches := m.slashMatches()
	if len(matches) == 0 {
		return false
	}

	shared := commonPrefix(matches)
	if len(matches) == 1 {
		// Nothing left to choose, so the argument can be started.
		m.input.SetValue("/" + shared + " ")
	} else {
		m.input.SetValue("/" + shared)
	}
	m.input.CursorEnd()
	return true
}

// commonPrefix is the longest start every candidate shares.
func commonPrefix(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}

	shared := candidates[0]
	for _, candidate := range candidates[1:] {
		for !strings.HasPrefix(candidate, shared) {
			shared = shared[:len(shared)-1]
			if shared == "" {
				return ""
			}
		}
	}
	return shared
}

// slashHint lists the commands still matching, for the row above the prompt.
func (m Model) slashHint(width int) string {
	matches := m.slashMatches()
	if len(matches) == 0 {
		return ""
	}

	var shown []string
	used := 0
	for _, command := range matches {
		entry := "/" + command
		if used+len(entry)+2 > width-6 {
			shown = append(shown, "…")
			break
		}
		shown = append(shown, entry)
		used += len(entry) + 2
	}
	return strings.Join(shown, "  ")
}
