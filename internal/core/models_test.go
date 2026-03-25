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
