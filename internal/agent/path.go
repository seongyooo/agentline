package agent

import (
	"path/filepath"
	"strings"
)

// RelativePath converts a path an agent reported into the root-relative slash
// path the event model and the project tree use.
//
// Agents report whatever their platform calls a path: a full Windows path, a
// Unix one, sometimes one already relative to the project. The rest of
// AgentLine works in one form, so every adapter converts at its edge.
//
// A path outside the project returns empty and is dropped by the caller. That
// rule lives here rather than in each adapter so it cannot drift between them:
// an adapter that quietly kept such a path would show a file under a name it
// does not have, and one that resolved it would report activity somewhere the
// user is not looking.
func RelativePath(root, path string) string {
	if root == "" || path == "" {
		return ""
	}

	// Already relative, which some agents report for files inside the project.
	if !filepath.IsAbs(path) {
		return cleanRelative(path)
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	return cleanRelative(rel)
}

// cleanRelative normalises separators and rejects anything above the root.
func cleanRelative(path string) string {
	path = filepath.ToSlash(path)
	if path == ".." || strings.HasPrefix(path, "../") {
		return ""
	}
	return path
}
