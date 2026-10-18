//go:build ortools && !windows

package solver

import (
	"testing"
	"time"
)

// BenchmarkOrtools5Classes меряет стоимость СБОРКИ модели (тайм-лимит 1с —
// wall_time из bind.cpp = сборка + ~1с поиска).
func BenchmarkOrtools5Classes(b *testing.B) {
	in := synthSchool(5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ortoolsSolve(in, 2, 1*time.Second)
	}
}
