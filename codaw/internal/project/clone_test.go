package project

import "testing"

// TestCloneIsDeep verifies every mutable level is actually copied — sharing
// any of these between snapshots would reintroduce the data race that
// copy-on-write exists to prevent.
func TestCloneIsDeep(t *testing.T) {
	orig := &Project{
		Master: &Master{Gain: 0, FX: []FX{{Type: "eq_3band", Params: map[string]any{"low_db": 1.5}}}},
		Buses:  []*Bus{{ID: "drums", Gain: -1.5}},
		Tracks: []*Track{{
			ID:    "kick",
			Gain:  0,
			FX:    []FX{{Type: "eq_3band", Params: map[string]any{"low_db": 3.0}}},
			Clips: []Clip{{File: "k.wav", Start: 0, End: 4, Gain: 0}},
			Automation: []Automation{{
				Target: "gain",
				Points: []AutomationPoint{{Beat: 0, Value: -24}, {Beat: 16, Value: 6}},
			}},
		}},
	}

	c := orig.Clone()

	// Mutate every level of the clone; the original must not move.
	c.Master.Gain = -60
	c.Master.FX[0].Params["low_db"] = 99.0
	c.Buses[0].Gain = -60
	c.Tracks[0].Gain = -60
	c.Tracks[0].FX[0].Params["low_db"] = 99.0
	c.Tracks[0].Clips[0].Gain = -60
	c.Tracks[0].Automation[0].Points[0].Value = 99

	if orig.Master.Gain != 0 {
		t.Error("master not deep-copied")
	}
	if orig.Master.FX[0].Params["low_db"] != 1.5 {
		t.Error("master fx params map shared between clone and original")
	}
	if orig.Buses[0].Gain != -1.5 {
		t.Error("bus not deep-copied")
	}
	if orig.Tracks[0].Gain != 0 {
		t.Error("track not deep-copied")
	}
	if orig.Tracks[0].FX[0].Params["low_db"] != 3.0 {
		t.Error("track fx params map shared")
	}
	if orig.Tracks[0].Clips[0].Gain != 0 {
		t.Error("clips slice shared")
	}
	if orig.Tracks[0].Automation[0].Points[0].Value != -24 {
		t.Error("automation points slice shared")
	}
}

func TestCloneNil(t *testing.T) {
	var p *Project
	if p.Clone() != nil {
		t.Error("Clone of nil should be nil")
	}
}
