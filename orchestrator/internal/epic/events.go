package epic

import "sync"

type EpicEventType string

const (
	BeadStateChanged EpicEventType = "BeadStateChanged"
	SessionStarted   EpicEventType = "SessionStarted"
	SessionError     EpicEventType = "SessionError"
	WaveAdvanced     EpicEventType = "WaveAdvanced"
)

type EpicEvent struct {
	Type      EpicEventType `json:"type"`
	EpicID    string        `json:"epicId"`
	BeadID    string        `json:"beadId"`
	SessionID string        `json:"sessionId"`
	Detail    string        `json:"detail"`
	Time      int64         `json:"time"`
}

type ParallelismMetric struct {
	Peak     int     `json:"peak"`
	GraphMax int     `json:"graphMax"`
	Realized float64 `json:"realized"`
}

type ParallelismTracker struct {
	mu       sync.Mutex
	active   map[string]bool
	current  int
	peak     int
	graphMax int
}

func NewParallelismTracker(graphMax int) *ParallelismTracker {
	return &ParallelismTracker{
		active:   make(map[string]bool),
		graphMax: graphMax,
	}
}

func (pt *ParallelismTracker) Started(id string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if pt.active[id] {
		return
	}
	pt.active[id] = true
	pt.current++
	if pt.current > pt.peak {
		pt.peak = pt.current
	}
}

func (pt *ParallelismTracker) Finished(id string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if !pt.active[id] {
		return
	}
	delete(pt.active, id)
	pt.current--
}

func (pt *ParallelismTracker) Metric() ParallelismMetric {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	realized := float64(0)
	if pt.graphMax > 0 {
		realized = float64(pt.peak) / float64(pt.graphMax)
	}
	return ParallelismMetric{
		Peak:     pt.peak,
		GraphMax: pt.graphMax,
		Realized: realized,
	}
}
