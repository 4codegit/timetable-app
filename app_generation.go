package main

import (
	"runtime"
	"sync"
	"time"

	"timetable/internal/domain"
	"timetable/internal/solver"
)

// generationState tracks live progress of an in-progress precise generation.
type generationState struct {
	mu     sync.RWMutex
	states map[int64]*GenProgress
	nextID int64
}

type GenProgress struct {
	ID      int64  `json:"id"`
	Placed  int    `json:"placed"`
	Total   int    `json:"total"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

var genState = &generationState{
	states: make(map[int64]*GenProgress),
}

func (g *generationState) newID() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextID++
	return g.nextID
}

func (g *generationState) set(id int64, placed, total int, status string, err string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if s, ok := g.states[id]; ok {
		s.Placed = placed
		s.Total = total
		s.Status = status
		s.Error = err
	}
}

func (g *generationState) get(id int64) (GenProgress, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	s, ok := g.states[id]
	if !ok {
		return GenProgress{}, false
	}
	return *s, true
}

func (g *generationState) delete(id int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.states, id)
}

// StartPreciseGeneration launches OR-Tools CP-SAT (or pure-Go fallback) in a
// background goroutine and returns a generation ID immediately. The frontend
// polls GetGenerationProgress to show a live progress bar.
// solveTimeoutSec == 0 means no time limit — solver runs until finished.
func (a *App) StartPreciseGeneration(schoolID, days, slots, daysMask, solveTimeoutSec int) int64 {
	in, err := a.loadSolveInput(schoolID, days, slots, daysMask)
	if err != nil {
		id := genState.newID()
		genState.set(id, 0, 0, "error", err.Error())
		return id
	}

	timeout := time.Duration(solveTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 0 // без лимита — solver работает до завершения
	}

	id := genState.newID()
	total := 0
	for _, l := range in.Lessons {
		if h := l.HoursPerWeek; h > 0 {
			total += h
		}
	}
	genState.set(id, 0, total, "running", "")

	go func() {
		res := solver.SolvePrecise(a.ctx, in, runtime.NumCPU(), timeout, func(placed int) {
			genState.set(id, placed, total, "running", "")
		})
		if err := a.store.ReplaceSchedule(schoolID, res.Entries); err != nil {
			genState.set(id, res.Placed, res.Total, "error", err.Error())
			return
		}
		genState.set(id, res.Placed, res.Total, "done", "")
	}()

	return id
}

// GetGenerationProgress returns the current progress of a running generation.
// The frontend polls this to show a live % progress bar.
func (a *App) GetGenerationProgress(id int64) GenProgress {
	p, ok := genState.get(id)
	if !ok {
		return GenProgress{ID: id, Status: "error", Error: "generation not found"}
	}
	return p
}

// Generate runs the CSP solver and persists the result.
func (a *App) Generate(schoolID, days, slots, daysMask int) (*solver.Result, error) {
	in, err := a.loadSolveInput(schoolID, days, slots, daysMask)
	if err != nil {
		return nil, err
	}
	res := solver.Solve(a.ctx, in, runtime.NumCPU(), 30*time.Second, nil)

	err = a.store.ReplaceSchedule(schoolID, res.Entries)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// HasPreciseSolver reports whether the running binary was compiled with the
// OR-Tools CP-SAT solver. The frontend uses this to display an accurate badge
// instead of falsely claiming "CP-SAT enabled" when the pure-Go fallback runs.
func (a *App) HasPreciseSolver() bool {
	return solver.HasPreciseSolver()
}

// loadSolveInput собирает SolveInput из хранилища (общее для Generate и
// StartPreciseGeneration).
func (a *App) loadSolveInput(schoolID, days, slots, daysMask int) (solver.SolveInput, error) {
	lessons, err := a.store.ListLessons(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
	ts, _ := a.store.ListTeachers(schoolID)
	cs, _ := a.store.ListClasses(schoolID)
	rs, _ := a.store.ListRooms(schoolID)
	subs, _ := a.store.ListSubjects(schoolID)
	cons, _ := a.store.ListConstraints(schoolID)
	return solver.SolveInput{
		SchoolID:    schoolID,
		Lessons:     lessons,
		Teachers:    toTeacherMap(ts),
		Classes:     toClassMap(cs),
		Rooms:       rs,
		Subjects:    toSubjMap(subs),
		Constraints: cons,
		Config: domain.SchedulingConfig{
			DaysPerWeek: days,
			SlotsPerDay: slots,
			DaysMask:    daysMask,
		},
	}, nil
}
