package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}

// repo builds a real repository with one commit, so the tests exercise the
// actual porcelain output rather than a fixture that could drift from it.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()

	write(t, root, "committed.txt", "one")
	write(t, root, "src/tracked.go", "package src")

	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
		{"add", "."},
		{"commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

func write(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCleanRepositoryReportsBranch(t *testing.T) {
	got := Load(ctx(t), repo(t))

	if got.Branch != "main" {
		t.Errorf("Branch = %q, want main", got.Branch)
	}
	if got.Detached {
		t.Error("Detached = true on a branch checkout")
	}
	if got.Dirty() {
		t.Errorf("Dirty = true on a clean tree: %v", got.Files)
	}
}

func TestModifiedAndUntrackedFiles(t *testing.T) {
	root := repo(t)
	write(t, root, "committed.txt", "changed")
	write(t, root, "brand-new.txt", "new")
	write(t, root, "src/added.go", "package src")

	got := Load(ctx(t), root)

	if !got.Dirty() {
		t.Fatal("Dirty = false with changes present")
	}
	tests := []struct {
		path string
		want FileStatus
	}{
		{"committed.txt", Modified},
		{"brand-new.txt", Untracked},
		{"src/added.go", Untracked},
		{"src/tracked.go", Unchanged},
		{"never-existed.txt", Unchanged},
	}
	for _, tc := range tests {
		if got := got.Of(tc.path); got != tc.want {
			t.Errorf("Of(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// Paths must be slash-separated regardless of platform, because the tree and
// the event model both use that form.
func TestPathsUseForwardSlashes(t *testing.T) {
	root := repo(t)
	write(t, root, "src/tracked.go", "package changed")

	if got := Load(ctx(t), root); got.Of("src/tracked.go") != Modified {
		t.Errorf("nested path not reported: %v", got.Files)
	}
}

// Staged and unstaged changes are both just "modified": telling them apart
// would only matter to a Git UI.
func TestStagedChangesAreModified(t *testing.T) {
	root := repo(t)
	write(t, root, "committed.txt", "staged change")
	if out, err := exec.Command("git", "-C", root, "add", "committed.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	if got := Load(ctx(t), root); got.Of("committed.txt") != Modified {
		t.Errorf("staged file = %v, want Modified", got.Of("committed.txt"))
	}
}

// A rename is followed by its source path in the porcelain stream; that extra
// record must not be mistaken for another entry.
func TestRenameReportsOnlyTheNewPath(t *testing.T) {
	root := repo(t)
	for _, args := range [][]string{{"mv", "committed.txt", "renamed.txt"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	got := Load(ctx(t), root)
	if got.Of("renamed.txt") != Modified {
		t.Errorf("renamed.txt = %v, want Modified", got.Of("renamed.txt"))
	}
	for path := range got.Files {
		if path == "" {
			t.Error("empty path parsed from the rename record")
		}
	}
}

// AgentLine may be rooted below the repository root; paths must be reported
// relative to what it is showing, and anything above it dropped.
func TestPathsAreRelativeToTheShownDirectory(t *testing.T) {
	root := repo(t)
	write(t, root, "committed.txt", "changed at top")
	write(t, root, "src/tracked.go", "package changed")

	got := Load(ctx(t), filepath.Join(root, "src"))

	if got.Of("tracked.go") != Modified {
		t.Errorf("tracked.go = %v, want Modified", got.Of("tracked.go"))
	}
	if _, ok := got.Files["committed.txt"]; ok {
		t.Error("a change above the shown directory leaked in")
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want main", got.Branch)
	}
}

// A brand-new directory is where seeing what is new matters most, so its
// files must be listed individually rather than collapsed into one entry.
func TestFilesInsideANewDirectoryAreListed(t *testing.T) {
	root := repo(t)
	write(t, root, "internal/fresh/one.go", "package fresh")
	write(t, root, "internal/fresh/two.go", "package fresh")

	got := Load(ctx(t), root)

	for _, path := range []string{"internal/fresh/one.go", "internal/fresh/two.go"} {
		if got.Of(path) != Untracked {
			t.Errorf("Of(%q) = %v, want Untracked", path, got.Of(path))
		}
	}
	for path := range got.Files {
		if strings.HasSuffix(path, "/") {
			t.Errorf("path %q kept its trailing slash", path)
		}
	}
}

// A repository with no commits still has a branch name to report.
func TestRepositoryWithNoCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	write(t, root, "first.txt", "hello")

	got := Load(ctx(t), root)
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want main", got.Branch)
	}
	if got.Of("first.txt") != Untracked {
		t.Errorf("first.txt = %v, want Untracked", got.Of("first.txt"))
	}
}

func TestDetachedHead(t *testing.T) {
	root := repo(t)
	if out, err := exec.Command("git", "-C", root, "checkout", "--detach", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach: %v\n%s", err, out)
	}

	got := Load(ctx(t), root)
	if !got.Detached {
		t.Error("Detached = false after detaching HEAD")
	}
	if got.Branch != "" {
		t.Errorf("Branch = %q, want empty when detached", got.Branch)
	}
}

// Not being a repository is normal, not a failure.
func TestNonRepositoryIsEmptyNotAnError(t *testing.T) {
	got := Load(ctx(t), t.TempDir())

	if got.Available() {
		t.Errorf("Available = true outside a repository: %+v", got)
	}
	if got.Dirty() || got.Branch != "" {
		t.Errorf("got %+v, want the zero Status", got)
	}
	if got.Of("anything.go") != Unchanged {
		t.Error("Of() on the zero Status should be Unchanged")
	}
}

// A cancelled context must not hang or panic.
func TestCancelledContextReturnsEmpty(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	if got := Load(cancelled, repo(t)); got.Available() {
		t.Errorf("got %+v, want the zero Status", got)
	}
}
