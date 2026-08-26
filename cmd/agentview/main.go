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
	mission := flag.String("mission", "", "the high-level goal shown in the MISSION panel")
	interval := flag.Duration("mock-interval", 2*time.Second, "delay between mock events")
	flag.Parse()

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
	st.Agent.Mission = *mission
	st.Project.Tree = scanner.NewTree()

	// Until a Claude Code adapter exists, the UI is driven by a mock source
	// replaying activity over files that actually exist in this project.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := startMockSource(ctx, scanner, st.Project.Tree, *interval)
	if err != nil {
		log.Printf("no event source: %v", err)
	}

	log.Printf("starting agentview in %s", root)
	_, err = tea.NewProgram(tui.New(st, scanner, stream), tea.WithAltScreen()).Run()
	return err
}

// startMockSource replays sample activity over real project files. It returns
// a nil stream when the project has no files to act on, leaving a static UI
// rather than inventing paths that do not exist.
func startMockSource(ctx context.Context, scanner *project.Scanner, tree *project.Node, interval time.Duration) (<-chan events.Event, error) {
	src := &agent.Mock{
		Paths:    sampleFiles(scanner, tree, 3),
		Interval: interval,
		Loop:     true,
	}
	return src.Events(ctx)
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
