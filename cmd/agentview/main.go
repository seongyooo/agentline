// Command agentview is a situational-awareness UI for AI coding agents.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/seonl/agentview/internal/agent"
	"github.com/seonl/agentview/internal/agent/claude"
	"github.com/seonl/agentview/internal/events"
	"github.com/seonl/agentview/internal/project"
	"github.com/seonl/agentview/internal/tui"

	"github.com/seonl/agentview/internal/state"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agentview:", err)
		os.Exit(1)
	}
}

func run() error {
	rootFlag := flag.String("root", "", "project root (default: detected from the working directory)")
	mission := flag.String("mission", "", "pin the MISSION panel to this goal instead of deriving it from the agent's prompts")
	run := flag.Bool("run", false, "launch and own a Claude Code session, making the prompt box live")
	sourceName := flag.String("source", "claude", `event source when not using --run: "claude" or "mock"`)
	addr := flag.String("addr", claude.DefaultAddr, "address to receive Claude Code hooks on")
	interval := flag.Duration("mock-interval", 2*time.Second, "delay between mock events")
	printHooks := flag.Bool("print-hooks", false, "print the hook settings to install in the observed project, then exit")
	flag.Parse()

	if *printHooks {
		settings, err := claude.HookSettings(*addr)
		if err != nil {
			return fmt.Errorf("render hook settings: %w", err)
		}
		fmt.Println(string(settings))
		return nil
	}

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		return err
	}

	closeLog, err := setupLogging()
	if err != nil {
		return err
	}
	defer closeLog()

	scanner := project.NewScanner(root)
	st := state.New(root)
	st.PinMission(*mission)
	st.Project.Tree = scanner.NewTree()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := tui.New(st, scanner, nil)

	var stream <-chan events.Event
	hint := ""

	if *run {
		// AgentView owns the session, so the prompt box can submit to it.
		session := claude.NewStream(root)
		stream, err = session.Events(ctx)
		if err == nil {
			model = model.WithSender(session)
			hint = "Session ready — press tab to send a prompt"
		}
	} else {
		stream, err = startSource(ctx, *sourceName, sourceConfig{
			root:     root,
			addr:     *addr,
			scanner:  scanner,
			tree:     st.Project.Tree,
			interval: *interval,
		})
		hint = sourceHint(*sourceName, *addr)
	}

	if err != nil {
		// A UI with no source still shows the project. Say why it is idle
		// rather than leaving it looking broken.
		log.Printf("no event source: %v", err)
		hint = fmt.Sprintf("Event source unavailable: %v", err)
	}

	log.Printf("starting agentview in %s", root)
	// The cursor is placed after each frame so an input method composes where
	// the text is. Without it Korean appears in the corner of the screen
	// until the syllable is finished.
	caret := &tui.Caret{}

	program := tea.NewProgram(
		model.WithStream(stream).WithHint(hint).WithCaret(caret),
		tea.WithAltScreen(),
		// Cells only, so the terminal's own text selection still works: a
		// full motion grab would take copy-paste away from the user.
		tea.WithMouseCellMotion(),
		tea.WithOutput(caret.Writer(os.Stdout)),
	)
	_, err = program.Run()
	return err
}

// sourceHint tells the user where activity is expected from, so a setup that
// is not wired up can be told apart from an agent that is simply idle.
func sourceHint(name, addr string) string {
	if name == "claude" {
		return fmt.Sprintf("Waiting for Claude Code hooks on %s", addr)
	}
	return fmt.Sprintf("Waiting for the %s source", name)
}

// sourceConfig carries what the event sources need to start.
type sourceConfig struct {
	root     string
	addr     string
	scanner  *project.Scanner
	tree     *project.Node
	interval time.Duration
}

// startSource opens the requested event source.
func startSource(ctx context.Context, name string, cfg sourceConfig) (<-chan events.Event, error) {
	switch name {
	case "claude":
		return claude.New(cfg.root, cfg.addr).Events(ctx)

	case "mock":
		// Replays sample activity over files that actually exist, so nothing
		// on screen refers to a path this project does not have.
		src := &agent.Mock{
			Paths:    sampleFiles(cfg.scanner, cfg.tree, 3),
			Interval: cfg.interval,
			Loop:     true,
		}
		return src.Events(ctx)
	}
	return nil, fmt.Errorf("unknown source %q", name)
}

// resolveRoot honors an explicit --root, otherwise detects the project root
// by walking up from the working directory.
func resolveRoot(override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve --root: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("project root: %w", err)
		}
		return abs, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return project.FindRoot(cwd), nil
}

// setupLogging sends diagnostics to a file so they never corrupt the TUI.
func setupLogging() (func(), error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "agentview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(filepath.Join(dir, "agentview.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags)
	return func() { f.Close() }, nil
}

// sampleFiles collects up to n real file paths, preferring shallow ones so
// the revealed tree stays compact.
func sampleFiles(scanner *project.Scanner, root *project.Node, n int) []string {
	var found []string
	var walk func(*project.Node, int)

	walk = func(node *project.Node, depth int) {
		if node == nil || len(found) >= n || depth > 3 {
			return
		}
		scanner.Load(node)
		for _, c := range node.Children {
			if len(found) >= n {
				return
			}
			if !c.Dir && !c.Placeholder {
				found = append(found, c.Path)
			}
		}
		for _, c := range node.Children {
			if c.Dir && !c.Placeholder {
				walk(c, depth+1)
			}
		}
	}

	walk(root, 0)
	return found
}
