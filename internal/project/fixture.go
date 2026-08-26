package project

// MockTree returns the sample project from IMPLEMENTATION.md as a fixed,
// filesystem-free tree. It is a test fixture shared across packages; the
// application itself builds its tree with a Scanner.
func MockTree() *Node {
	root := Dir("Assets",
		Dir("Scripts",
			Dir("Core"),
			Dir("Player"),
			Dir("Puzzle",
				File("Valve.cs"),
				File("DrainSystem.cs"),
			),
			Dir("Rooms",
				File("WaterRoom.cs"),
			),
		),
		Dir("Prefabs"),
		Dir("Scenes"),
		Dir("Materials"),
	)
	AssignPaths(root, "")
	return root
}
