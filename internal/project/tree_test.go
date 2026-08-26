package project

import (
	"strings"
	"testing"
)

func TestAssignPathsBuildsRelativePaths(t *testing.T) {
	root := MockTree()

	var found bool
	for _, row := range Flatten(root) {
		if row.Node.Name == "DrainSystem.cs" {
			found = true
			if want := "Assets/Scripts/Puzzle/DrainSystem.cs"; row.Node.Path != want {
				t.Errorf("Path = %q, want %q", row.Node.Path, want)
			}
		}
	}
	if !found {
		t.Fatal("DrainSystem.cs not in flattened tree")
	}
}

func TestFlattenDrawsBranches(t *testing.T) {
	root := Dir("a", Dir("b", File("c.go")), File("d.go"))
	AssignPaths(root, "")

	var got []string
	for _, row := range Flatten(root) {
		got = append(got, row.Prefix+row.Node.Name)
	}

	want := []string{"a", "├─ b", "│  └─ c.go", "└─ d.go"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("tree =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFlattenSkipsCollapsedChildren(t *testing.T) {
	root := Dir("a", Dir("b", File("c.go")))
	AssignPaths(root, "")
	root.Children[0].Expanded = false

	if got := len(Flatten(root)); got != 2 {
		t.Errorf("got %d rows, want 2 (collapsed child hidden)", got)
	}
}

func TestFlattenNilTree(t *testing.T) {
	if got := Flatten(nil); got != nil {
		t.Errorf("Flatten(nil) = %v, want nil", got)
	}
}
