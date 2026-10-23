package main

import "sync"

type GenProgress struct {
	ID      int64  `json:"id"`
	Placed  int    `json:"placed"`
	Total   int    `json:"total"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type generationState struct {
	mu     sync.RWMutex
	states map[int64]*GenProgress
	nextID int64
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
