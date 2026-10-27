package main

import (
	"runtime"
	"sync"
	"time"

	"timetable/internal/domain"
	"timetable/internal/solver"
)

// generationState tracks live progress of in-progress precise generations.
// Terminal entries are reaped lazily so the map cannot grow unbounded while
// still giving the frontend a grace window to read the final status.
type generationState struct {
	mu     sync.RWMutex
	states map[int64]*GenProgress
	nextID int64
}

type GenProgress struct {
	ID     int64  `json:"id"`
	Placed int    `json:"placed"`
	Total  int    `json:"total"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`

	schoolID int       `json:"-"`
	doneAt   time.Time `json:"-"`
}

// reapDelay — сколько терминальная запись живёт после done/error, прежде чем
// её можно удалить (фронтенд поллит каждые 500 мс и должен успеть прочитать).
const reapDelay = 30 * time.Second

var genState = &generationState{
	states: make(map[int64]*GenProgress),
}

// begin резервирует запись "running" для школы. Если генерация этой школы уже
// идёт — возвращает её ID и false (защита от параллельных запусков: второй
// вызов просто цепляется к прогрессу первого).
func (g *generationState) begin(schoolID int) (int64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reapLocked()
	for _, s := range g.states {
		if s.schoolID == schoolID && s.Status == "running" {
			return s.ID, false
		}
	}
	g.nextID++
	id := g.nextID
	g.states[id] = &GenProgress{ID: id, schoolID: schoolID, Status: "running"}
	return id, true
}

// set обновляет запись, создавая её при необходимости (раньше был update-only,
// из-за чего прогресс никогда не записывался и поллинг всегда получал
// "generation not found").
func (g *generationState) set(id int64, placed, total int, status string, err string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.states[id]
	if !ok {
		s = &GenProgress{ID: id, Status: status}
		g.states[id] = s
	}
	s.Placed = placed
	s.Total = total
	s.Status = status
	s.Error = err
	if status == "done" || status == "error" {
		s.doneAt = time.Now()
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

// reapLocked удаляет терминальные записи старше reapDelay. Вызывается под g.mu.
func (g *generationState) reapLocked() {
	now := time.Now()
	for id, s := range g.states {
		if !s.doneAt.IsZero() && now.Sub(s.doneAt) > reapDelay {
			delete(g.states, id)
		}
	}
}

// StartPreciseGeneration launches OR-Tools CP-SAT (or pure-Go fallback) in a
// background goroutine and returns a generation ID immediately. The frontend
// polls GetGenerationProgress to show a live progress bar.
// solveTimeoutSec == 0 means no time limit — solver runs until finished.
func (a *App) StartPreciseGeneration(schoolID, days, slots, daysMask, solveTimeoutSec int) int64 {
	id, isNew := genState.begin(schoolID)
	if !isNew {
		return id // уже идёт генерация этой школы — цепляемся к её прогрессу
	}

	in, err := a.loadSolveInput(schoolID, days, slots, daysMask)
	if err != nil {
		genState.set(id, 0, 0, "error", err.Error())
		return id
	}

	timeout := time.Duration(solveTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 0 // без лимита — solver работает до завершения
	}
	// The pure-Go fallback has no cancellation control in the Wails UI. If a
	// large school auto-selects this path with "no limit", keep it bounded so
	// an incomplete instance cannot run forever in the background.
	if !solver.HasPreciseSolver() && timeout == 0 {
		timeout = 30 * time.Second
	}

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
// StartPreciseGeneration). Ошибка любого списка — ошибка всей загрузки:
// запускать солвер на неполных данных нельзя.
func (a *App) loadSolveInput(schoolID, days, slots, daysMask int) (solver.SolveInput, error) {
	lessons, err := a.store.ListLessons(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
	ts, err := a.store.ListTeachers(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
	cs, err := a.store.ListClasses(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
	rs, err := a.store.ListRooms(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
	subs, err := a.store.ListSubjects(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
	cons, err := a.store.ListConstraints(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
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
