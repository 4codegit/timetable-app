//go:build !ortools

package main

import "testing"

// TestHasPreciseSolver ensures the boolean is exposed at the App layer so the
// frontend can show an accurate "OR-Tools available" badge. The default build
// (no -tags ortools) must report false; сборка с -tags ortools проверяет сам
// биндинг в internal/solver (ortools_csat_test.go).
func TestHasPreciseSolver(t *testing.T) {
	a := newTestApp(t)
	if a.HasPreciseSolver() {
		t.Fatal("default build (no -tags ortools) should report HasPreciseSolver=false")
	}
}
