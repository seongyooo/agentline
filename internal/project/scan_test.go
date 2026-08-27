package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleRepo builds a throwaway project containing source, a nested package,
// and the kinds of directories the scanner is expected to hide.
func sampleRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	dirs := []string{"src/core", "node_modules/left-pad", ".git/objects", "build"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := []string{"go.mod", "README.md", "src/main.go", "src/core/engine.go", "build/out.bin"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func childNames(n *Node) []string {
	names := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		names = append(names, c.Name)
	}
	return names
}

func TestScannerHidesNoiseAndDotEntries(t *testing.T) {
	tree := NewScanner(sampleRepo(t)).NewTree()

	// Directories first, then files ordered case-insensitively.
	got := strings.Join(childNames(tree), ",")
	if want := "src,go.mod,README.md"; got != want {
		t.Errorf("root children = %q, want %q", got, want)
	}
}

func TestScannerSortsDirectoriesFirst(t *testing.T) {
	nodes := []*Node{File("zebra.go"), Dir("Beta"), File("Alpha.go"), Dir("alpha")}
	sortNodes(nodes)

	got := strings.Join([]string{nodes[0].Name, nodes[1].Name, nodes[2].Name, nodes[3].Name}, ",")
	if want := "alpha,Beta,Alpha.go,zebra.go"; got != want {
		t.Errorf("sorted = %q, want %q", got, want)
	}
}

func TestScannerLoadsLazily(t *testing.T) {
	tree := NewScanner(sampleRepo(t)).NewTree()

	src := tree.Children[0]
	if src.Name != "src" {
		t.Fatalf("first child = %q, want src", src.Name)
	}
	if src.Loaded || len(src.Children) != 0 {
		t.Error("subdirectory was read before it was expanded")
	}
}

func TestLoadIsIdempotent(t *testing.T) {
	root := sampleRepo(t)
	s := NewScanner(root)
	tree := s.NewTree()

	src := tree.Children[0]
	s.Load(src)
	first := len(src.Children)

	// A second Load must not duplicate children, even if the directory changed.
	if err := os.WriteFile(filepath.Join(root, "src", "extra.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Load(src)

	if got := len(src.Children); got != first {
		t.Errorf("children = %d after reload, want %d", got, first)
	}
}

func TestLoadRecordsUnreadableDirectory(t *testing.T) {
	s := NewScanner(sampleRepo(t))
	missing := &Node{Name: "ghost", Path: "ghost", Dir: true}

	s.Load(missing)

	if missing.Err == nil {
		t.Error("Err not set for a directory that cannot be read")
	}
	if !missing.Loaded {
		t.Error("failed load must still mark the node Loaded so it is not retried")
	}
}

func TestCapLeavesPlaceholderForOmittedEntries(t *testing.T) {
	s := &Scanner{Root: t.TempDir(), MaxEntries: 2}
	got := s.cap([]*Node{File("a"), File("b"), File("c"), File("d")})

	if len(got) != 3 {
		t.Fatalf("got %d entries, want 2 kept plus a placeholder", len(got))
	}
	last := got[len(got)-1]
	if !last.Placeholder {
		t.Fatal("last entry is not a placeholder")
	}
	if !strings.Contains(last.Name, "2") {
		t.Errorf("placeholder = %q, want it to report 2 omitted entries", last.Name)
	}
}

func TestRevealExpandsAncestors(t *testing.T) {
	s := NewScanner(sampleRepo(t))
	tree := s.NewTree()

	node := s.Reveal(tree, "src/core/engine.go")
	if node == nil {
		t.Fatal("Reveal returned nil for a path that exists")
	}
	if node.Name != "engine.go" {
		t.Errorf("revealed %q, want engine.go", node.Name)
	}

	for _, want := range []string{"src", "src/core"} {
		dir := Find(tree, want)
		if dir == nil {
			t.Errorf("%q not in tree", want)
			continue
		}
		if !dir.Expanded {
			t.Errorf("%q was not expanded", want)
		}
	}
}

func TestRevealMissingPath(t *testing.T) {
	s := NewScanner(sampleRepo(t))
	if got := s.Reveal(s.NewTree(), "src/nope/gone.go"); got != nil {
		t.Errorf("Reveal = %v, want nil for a path that does not exist", got)
	}
}

func TestFindRootDetectsMarker(t *testing.T) {
	root := sampleRepo(t) // contains go.mod
	nested := filepath.Join(root, "src", "core")

	got, err := filepath.EvalSymlinks(FindRoot(nested))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("FindRoot = %q, want %q", got, want)
	}
}

// stubHome moves the ceiling the root search stops at.
func stubHome(t *testing.T, home string) {
	t.Helper()

	previous := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = previous })
}

// A stray marker in the home directory must not swallow the directory the user
// actually pointed at. This is a real report: one package.json left behind by
// an npm install run in the wrong terminal made every scratch folder on the
// machine resolve to the whole home directory.
func TestFindRootStopsAtHome(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubHome(t, home)

	scratch := filepath.Join(home, "Desktop", "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := FindRoot(scratch); !sameDir(got, scratch) {
		t.Errorf("FindRoot = %q, want %q", got, scratch)
	}
}

// The ceiling must not cost us the markers that are real. A project under home
// is the normal case, not the exception.
func TestFindRootPrefersAMarkerBelowHome(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubHome(t, home)

	project := filepath.Join(home, "code", "app")
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module app\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := FindRoot(filepath.Join(project, "src")); !sameDir(got, project) {
		t.Errorf("FindRoot = %q, want %q", got, project)
	}
}

// Everything downstream joins paths onto the root and compares them against
// absolute ones, so the fallback has to be absolute too — not whatever the
// caller happened to type.
func TestFindRootAnswersWithAnAbsolutePath(t *testing.T) {
	home := t.TempDir()
	stubHome(t, home)

	scratch := filepath.Join(home, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(scratch)

	if got := FindRoot("."); !filepath.IsAbs(got) {
		t.Errorf("FindRoot(%q) = %q, want an absolute path", ".", got)
	}
}

func TestFindRootFallsBackToStart(t *testing.T) {
	bare := t.TempDir() // no project marker anywhere above it in the temp tree
	if got := FindRoot(bare); got != bare && !strings.HasPrefix(bare, got) {
		t.Errorf("FindRoot = %q, want %q or an ancestor with a marker", got, bare)
	}
}
