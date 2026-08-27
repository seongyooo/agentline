// Package project models the project's filesystem structure. It knows nothing
// about agents or rendering.
package project

import (
	"path"
	"strings"
)

// Node is a file or directory in the project tree.
//
// Directories are populated lazily: Children is meaningful only once Loaded is
// set, so a large repository costs nothing until the user expands into it.
type Node struct {
	Name     string
	Path     string // slash-separated, relative to the project root
	Dir      bool
	Expanded bool
	Loaded   bool
	Err      error // set when the directory could not be read
	Children []*Node

	// Placeholder marks a synthetic row standing in for omitted entries. It
	// is not a real file and cannot be expanded.
	Placeholder bool
}

// Dir builds a directory node with its children already loaded.
func Dir(name string, children ...*Node) *Node {
	return &Node{Name: name, Dir: true, Expanded: true, Loaded: true, Children: children}
}

// File builds a file node.
func File(name string) *Node {
	return &Node{Name: name}
}

// AssignPaths fills in each node's Path from its position in the tree.
func AssignPaths(n *Node, parent string) {
	n.Path = n.Name
	if parent != "" {
		n.Path = path.Join(parent, n.Name)
	}
	for _, c := range n.Children {
		AssignPaths(c, n.Path)
	}
}

// Row is one rendered line of the tree: a node plus its box-drawing prefix.
type Row struct {
	Node   *Node
	Prefix string
}

// Flatten returns the visible rows of the tree, honoring Expanded.
func Flatten(root *Node) []Row {
	if root == nil {
		return nil
	}
	rows := []Row{{Node: root}}
	if root.Dir && root.Expanded {
		rows = append(rows, flattenChildren(root.Children, "")...)
	}
	return rows
}

func flattenChildren(children []*Node, indent string) []Row {
	var rows []Row
	for i, c := range children {
		branch, childIndent := "├─ ", indent+"│  "
		if i == len(children)-1 {
			branch, childIndent = "└─ ", indent+"   "
		}

		rows = append(rows, Row{Node: c, Prefix: indent + branch})
		if c.Dir && c.Expanded {
			rows = append(rows, flattenChildren(c.Children, childIndent)...)
		}
	}
	return rows
}

// Insert places a node for a root-relative path that is not on disk yet,
// creating any missing directories along the way.
//
// A file the agent has announced but not yet written has nothing for a scan to
// find, so it is put in the tree directly. It is ordinary from then on: the
// next refresh of its directory replaces it with what was actually written.
func Insert(root *Node, target string) *Node {
	if root == nil || target == "" {
		return nil
	}

	node := root
	segments := strings.Split(path.Clean(target), "/")
	for i, name := range segments {
		child := childByName(node, name)
		if child == nil {
			child = &Node{
				Name: name,
				Path: path.Join(node.Path, name),
				Dir:  i < len(segments)-1,
			}
			child.Loaded = child.Dir // nothing on disk to read for it yet
			node.Children = append(node.Children, child)
			sortNodes(node.Children)
		}
		node = child
	}
	return node
}

func childByName(n *Node, name string) *Node {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Find returns the node at a root-relative path, or nil if it is not loaded.
func Find(root *Node, target string) *Node {
	if root == nil {
		return nil
	}
	if root.Path == target {
		return root
	}
	for _, c := range root.Children {
		if n := Find(c, target); n != nil {
			return n
		}
	}
	return nil
}
