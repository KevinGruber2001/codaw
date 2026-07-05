package engine

// Pure-logic tests only: nothing here initialises an audio device, so these
// run fine in CI runners with no sound hardware.

import (
	"math"
	"testing"

	"github.com/KevinGruber2001/codaw/internal/project"
)

func TestBeatsToFrames(t *testing.T) {
	tr := transport{sampleRate: 48000, bpm: 120} // 120 BPM → 24000 frames/beat

	cases := []struct {
		beats float64
		want  uint64
	}{
		{0, 0},
		{-2, 0}, // negative clamps to 0
		{1, 24000},
		{8, 192000}, // beat 8 = 4 seconds
		{0.5, 12000},
	}
	for _, c := range cases {
		if got := tr.beatsToFrames(c.beats); got != c.want {
			t.Errorf("beatsToFrames(%v) = %d, want %d", c.beats, got, c.want)
		}
	}
}

func TestInterpolate(t *testing.T) {
	pts := []project.AutomationPoint{
		{Beat: 0, Value: -24},
		{Beat: 16, Value: 6},
	}

	cases := []struct {
		beat float64
		want float64
	}{
		{-1, -24}, // before first point → hold first
		{0, -24},
		{8, -9}, // halfway: -24 + 0.5*30
		{16, 6},
		{99, 6}, // past last point → hold last
	}
	for _, c := range cases {
		if got := interpolate(pts, c.beat); got != c.want {
			t.Errorf("interpolate(%v) = %v, want %v", c.beat, got, c.want)
		}
	}

	if got := interpolate(nil, 5); got != 0 {
		t.Errorf("interpolate(no points) = %v, want 0", got)
	}
	single := []project.AutomationPoint{{Beat: 4, Value: 1.5}}
	for _, beat := range []float64{0, 4, 10} {
		if got := interpolate(single, beat); got != 1.5 {
			t.Errorf("interpolate(single, %v) = %v, want 1.5", beat, got)
		}
	}
}

func TestTrackAudible(t *testing.T) {
	kick := &project.Track{ID: "kick"}
	pad := &project.Track{ID: "pad"}
	p := &project.Project{Tracks: []*project.Track{kick, pad}}

	if !trackAudible(p, kick) || !trackAudible(p, pad) {
		t.Error("no mute/solo: everything should be audible")
	}

	kick.Mute = true
	if trackAudible(p, kick) {
		t.Error("muted track should not be audible")
	}
	kick.Mute = false

	// Solo on pad silences kick, keeps pad.
	pad.Solo = true
	if trackAudible(p, kick) {
		t.Error("non-soloed track should be silent while another is soloed")
	}
	if !trackAudible(p, pad) {
		t.Error("soloed track should be audible")
	}

	// Mute wins over solo on the same track.
	pad.Mute = true
	if trackAudible(p, pad) {
		t.Error("muted+soloed track should be silent (mute wins)")
	}
}

func TestClipFilePosition(t *testing.T) {
	// A 4s file with a 1s in-point: playback walks 1→4, then (looping) 0→4…
	const offset, length = 1.0, 4.0

	cases := []struct {
		name    string
		loop    bool
		elapsed float64
		wantPos float64
		wantOK  bool
	}{
		{"start of clip", false, 0, 1.0, true},
		{"inside first pass", false, 2, 3.0, true},
		{"non-loop exhausted", false, 3.5, 0, false},
		{"loop wraps to file start", true, 3.5, 0.5, true},
		{"loop second wrap", true, 3 + 4 + 1.5, 1.5, true},
		{"zero-length file", true, 1, 0, false},
	}
	for _, c := range cases {
		length := length
		if c.name == "zero-length file" {
			length = 0
		}
		pos, ok := clipFilePosition(offset, length, c.loop, c.elapsed)
		if ok != c.wantOK || (ok && math.Abs(pos-c.wantPos) > 1e-9) {
			t.Errorf("%s: got (%v,%v), want (%v,%v)", c.name, pos, ok, c.wantPos, c.wantOK)
		}
	}
}

func TestParseTarget(t *testing.T) {
	if k, _, _, err := parseTarget("gain"); err != nil || k != autoGain {
		t.Errorf("gain: kind=%v err=%v", k, err)
	}
	if k, _, _, err := parseTarget("pan"); err != nil || k != autoPan {
		t.Errorf("pan: kind=%v err=%v", k, err)
	}
	k, idx, key, err := parseTarget("fx[2].wet")
	if err != nil || k != autoFX || idx != 2 || key != "wet" {
		t.Errorf("fx[2].wet: kind=%v idx=%d key=%q err=%v", k, idx, key, err)
	}
	for _, bad := range []string{"volume", "fx[x].wet", "fx[0]", "fx[0].", "fx["} {
		if _, _, _, err := parseTarget(bad); err == nil {
			t.Errorf("parseTarget(%q) should fail", bad)
		}
	}
}
