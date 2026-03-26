package core

import "testing"

func TestValidEffort(t *testing.T) {
	valid := []Effort{"", EffortXS, EffortS, EffortM, EffortL, EffortXL}
	for _, e := range valid {
		if !ValidEffort(e) {
			t.Errorf("expected %q to be valid", e)
		}
	}
	invalid := []Effort{"HUGE", "xs", "medium", "1"}
	for _, e := range invalid {
		if ValidEffort(e) {
			t.Errorf("expected %q to be invalid", e)
		}
	}
}

func TestValidPriority(t *testing.T) {
	valid := []Priority{"", PriorityP0, PriorityP1, PriorityP2, PriorityP3}
	for _, p := range valid {
		if !ValidPriority(p) {
			t.Errorf("expected %q to be valid", p)
		}
	}
	invalid := []Priority{"HIGH", "p0", "critical", "1", "A"}
	for _, p := range invalid {
		if ValidPriority(p) {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}
