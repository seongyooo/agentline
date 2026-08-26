package claude

import (
	"encoding/json"
	"fmt"
)

// hookEvents are the hooks AgentView asks Claude Code to deliver. Only what
// the adapter can act on is requested; subscribing to more would add latency
// to the agent's tool calls for payloads that would be thrown away.
var hookEvents = []string{
	hookUserPromptSubmit,
	hookPreToolUse,
	hookPostToolUse,
	hookPostToolUseFailure,
	hookStop,
	hookSessionEnd,
}

// HookSettings renders the settings.json fragment that points Claude Code's
// hooks at a running AgentView.
//
// The user installs this in the project they want observed. It is generated
// rather than documented so the address can never drift from what AgentView
// is actually listening on.
func HookSettings(addr string) ([]byte, error) {
	if addr == "" {
		addr = DefaultAddr
	}
	url := fmt.Sprintf("http://%s%s", addr, HookPath)

	hooks := map[string]any{}
	for _, event := range hookEvents {
		hooks[event] = []any{
			map[string]any{
				"hooks": []any{
					map[string]any{
						"type": "http",
						"url":  url,
						// Short: a hook runs inline with the agent's work, so
						// a stalled AgentView must not hold the agent up.
						"timeout": 5,
					},
				},
			},
		}
	}

	return json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
}
