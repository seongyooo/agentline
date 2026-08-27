package project

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// defaultMaxEntries caps how many children one directory contributes to the
// tree. AgentView shows where work is happening, not every file in a repo.
const defaultMaxEntries = 200

// rootMarkers identify a project root when walking up from the start
// directory, in the order they are checked at each level.
var rootMarkers = []string{".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml", ".hg"}

// noisyDirs are build and dependency outputs that would bury real source.
// Entries beginning with a dot are skipped separately.
var noisyDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"target":       true,
	"dist":         true,
	"build":        true,
	"out":          true,
	"obj":          true,
	"__pycache__":  true,
	"venv":         true,
	"Library":      true, // Unity
	"Temp":         true, // Unity
}

// FindRoot walks up from start looking for a project marker, falling back to
// start itself when none is found.
func FindRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return start
	}

	for {
		for _, marker := range rootMarkers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start // reached the filesystem root
		}
		dir = parent
	}
}

// Scanner reads project directories on demand.
type Scanner struct {
	Root       string // absolute path to the project root
	MaxEntries int    // per-directory cap; zero means the default
}

// NewScanner returns a Scanner rooted at an absolute path.
func NewScanner(root string) *Scanner {
	return &Scanner{Root: root, MaxEntries: defaultMaxEntries}
}

// NewTree returns the root node with its first level loaded.
func (s *Scanner) NewTree() *Node {
	root := &Node{Name: filepath.Base(s.Root), Dir: true, Expanded: true}
	s.Load(root)
	return root
}

// Load populates a directory's children. It is a no-op for files, for
// placeholders, and for directories that have already been read, so repeated
// expansion never re-hits the filesystem.
func (s *Scanner) Load(n *Node) {
	if n == nil || !n.Dir || n.Loaded || n.Placeholder {
		return
	}
	n.Loaded = true // a failed read must not be retried on every keypress

	entries, err := os.ReadDir(filepath.Join(s.Root, filepath.FromSlash(n.Path)))
	if err != nil {
		n.Err = err
		return
	}

	kept := make([]*Node, 0, len(entries))
	for _, e := range entries {
		if ignored(e.Name()) {
			continue
		}
		kept = append(kept, &Node{
			Name: e.Name(),
			Path: path.Join(n.Path, e.Name()),
			Dir:  e.IsDir(),
		})
	}

	sortNodes(kept)
	n.Children = s.cap(kept)
}

// cap trims an over-long directory, leaving a placeholder that reports how
// many entries were omitted rather than hiding them silently.
func (s *Scanner) cap(nodes []*Node) []*Node {
	limit := s.MaxEntries
	if limit <= 0 {
		limit = defaultMaxEntries
	}
	if len(nodes) <= limit {
		return nodes
	}

	omitted := len(nodes) - limit
	return append(nodes[:limit:limit], &Node{
		Name:        fmt.Sprintf("… %d more", omitted),
		Placeholder: true,
	})
}

// Refresh re-reads a directory that has already been loaded, picking up files
// created since. The tree is a snapshot of one scan, so without this a file
// the agent just wrote would never appear.
//
// Expansion state of the surviving children is preserved, so a refresh does
// not collapse what the user had opened.
func (s *Scanner) Refresh(n *Node) {
	if n == nil || !n.Dir || n.Placeholder {
		return
	}

	// Reloading replaces the child nodes, so every directory the user had
	// open below here is collected first and reopened afterwards. Without
	// this, one new file would collapse the whole subtree under the reader.
	open := map[string]bool{}
	collectExpanded(n, open)

	n.Loaded = false
	n.Children = nil
	s.Load(n)
	s.reopen(n, open)
}

// collectExpanded records the paths of every expanded directory in a subtree.
func collectExpanded(n *Node, into map[string]bool) {
	for _, c := range n.Children {
		if !c.Dir {
			continue
		}
		if c.Expanded {
			into[c.Path] = true
		}
		collectExpanded(c, into)
	}
}

// reopen re-expands the directories that were open before a reload.
func (s *Scanner) reopen(n *Node, open map[string]bool) {
	for _, c := range n.Children {
		if !c.Dir || !open[c.Path] {
			continue
		}
		c.Expanded = true
		s.Load(c)
		s.reopen(c, open)
	}
}

// RefreshParent re-reads the directory containing a root-relative path.
//
// The directory itself may be as new as the file, so this walks up to the
// nearest ancestor the tree already knows and reloads from there.
func (s *Scanner) RefreshParent(root *Node, target string) {
	for dir := path.Dir(target); ; dir = path.Dir(dir) {
		if dir == "." || dir == "/" || dir == "" {
			s.Refresh(root)
			return
		}
		if n := Find(root, dir); n != nil {
			s.Refresh(n)
			return
		}
	}
}

// Reveal expands every directory along a root-relative path so the node
// becomes visible, loading directories as it goes. It returns the node, or nil
// if the path does not exist.
func (s *Scanner) Reveal(root *Node, target string) *Node {
	if root == nil || target == "" {
		return root
	}

	node := root
	for _, name := range strings.Split(path.Clean(target), "/") {
		s.Load(node)
		node.Expanded = true

		child := childNamed(node, name)
		if child == nil {
			return nil
		}
		node = child
	}
	return node
}

func childNamed(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// sortNodes orders directories before files, then case-insensitively by name.
func sortNodes(nodes []*Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// ignored reports whether an entry is hidden from the tree.
func ignored(name string) bool {
	return strings.HasPrefix(name, ".") || noisyDirs[name]
}
