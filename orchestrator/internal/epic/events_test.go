package epic

import (
	"testing"
)

func TestParallelismTrackerRecordsPeakAndRealized(t *testing.T) {
	pt := NewParallelismTracker(3)
	pt.Started("a")
	pt.Started("b")
	pt.Finished("a")
	pt.Started("c")
	m := pt.Metric()
	if m.Peak != 2 {
		t.Errorf("Peak = %d, want 2", m.Peak)
	}
	if m.GraphMax != 3 {
		t.Errorf("GraphMax = %d, want 3", m.GraphMax)
	}
	want := float64(2) / float64(3)
	if m.Realized != want {
		t.Errorf("Realized = %f, want %f", m.Realized, want)
	}
}

func TestParallelismTrackerConcurrentZero(t *testing.T) {
	pt := NewParallelismTracker(5)
	pt.Started("a")
	pt.Finished("a")
	m := pt.Metric()
	if m.Peak != 1 {
		t.Errorf("Peak = %d, want 1", m.Peak)
	}
	want := float64(1) / float64(5)
	if m.Realized != want {
		t.Errorf("Realized = %f, want %f", m.Realized, want)
	}
}

func TestParallelismTrackerPeakHoldsAcrossWaves(t *testing.T) {
	pt := NewParallelismTracker(2)
	pt.Started("a")
	pt.Started("b")
	pt.Finished("a")
	pt.Started("c")
	pt.Finished("b")
	pt.Started("d")
	m := pt.Metric()
	if m.Peak != 2 {
		t.Errorf("Peak = %d, want 2", m.Peak)
	}
	if m.GraphMax != 2 {
		t.Errorf("GraphMax = %d, want 2", m.GraphMax)
	}
}
