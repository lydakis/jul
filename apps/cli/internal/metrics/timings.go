package metrics

import "time"

type Timings struct {
	TotalMs int64            `json:"total,omitempty"`
	PhaseMs map[string]int64 `json:"phase,omitempty"`
}

func NewTimings() Timings {
	return Timings{PhaseMs: map[string]int64{}}
}

func (t *Timings) Add(phase string, d time.Duration) {
	if t == nil {
		return
	}
	if t.PhaseMs == nil {
		t.PhaseMs = map[string]int64{}
	}
	t.PhaseMs[phase] = d.Milliseconds()
}
