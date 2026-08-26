package project

import (
	"os"
	"testing"
)

// BenchmarkNewTree measures the cost of the initial scan against a real
// directory. Set BENCH_ROOT to point it somewhere large.
func BenchmarkNewTree(b *testing.B) {
	root := os.Getenv("BENCH_ROOT")
	if root == "" {
		b.Skip("set BENCH_ROOT")
	}
	s := NewScanner(root)

	for b.Loop() {
		s.NewTree()
	}
}
