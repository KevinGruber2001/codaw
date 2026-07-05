package state

import (
	"sync"
	"testing"

	"github.com/KevinGruber2001/codaw/internal/project"
)

// testProject builds a minimal two-track project for store tests.
func testProject() *project.Project {
	return &project.Project{
		Transport: project.Transport{BPM: 120, SampleRate: 48000},
		Master:    &project.Master{Gain: 0},
		Tracks: []*project.Track{
			{ID: "kick", Gain: 0, Clips: []project.Clip{{File: "k.wav", Start: 0, End: 4}}},
			{ID: "pad", Gain: -6},
		},
	}
}

// TestApplySnapshotIsolation is the copy-on-write contract: a pointer obtained
// BEFORE a mutation must never observe the mutation. This is what makes it
// safe for the audio engine to keep reading a snapshot without holding locks.
func TestApplySnapshotIsolation(t *testing.T) {
	s := New(testProject())

	before := s.Get()
	s.Apply(SetTrackGain("kick", -30))
	after := s.Get()

	if got := before.Tracks[0].Gain; got != 0 {
		t.Errorf("old snapshot was mutated: kick gain = %v, want 0 (COW broken)", got)
	}
	if got := after.Tracks[0].Gain; got != -30 {
		t.Errorf("new snapshot missing mutation: kick gain = %v, want -30", got)
	}
	if before == after {
		t.Error("Apply must swap the project pointer, not mutate in place")
	}
}

// TestConcurrentApplyAndGet exists for the race detector: run it with
// `go test -race`. Before copy-on-write, readers iterating a snapshot while
// Apply mutated it in place was a data race; now it must be clean.
func TestConcurrentApplyAndGet(t *testing.T) {
	s := New(testProject())

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Reader: simulates the engine's automation ticker — grab a snapshot,
	// then read it repeatedly after the lock is long gone.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			p := s.Get()
			for _, tr := range p.Tracks {
				_ = tr.Gain
				for _, c := range tr.Clips {
					_ = c.Gain
				}
			}
		}
	}()

	// Writer: simulates the watcher applying mutations on file saves.
	for i := 0; i < 500; i++ {
		s.Apply(SetTrackGain("kick", float64(-i%40)))
		s.Apply(SetClipGain("kick", 0, float64(i%6)))
	}
	close(stop)
	wg.Wait()
}

// TestSetClipGain checks the targeted clip mutation: right event, right
// payload, out-of-range indices ignored.
func TestSetClipGain(t *testing.T) {
	s := New(testProject())

	ch := make(chan Event, 1)
	unsub := s.Subscribe(ch)
	defer unsub()

	s.Apply(SetClipGain("kick", 0, -3))
	ev := <-ch
	if ev.Type != EventClipGainChanged {
		t.Fatalf("event type = %s, want %s", ev.Type, EventClipGainChanged)
	}
	pl := ev.Payload.(ClipGainPayload)
	if pl.TrackID != "kick" || pl.ClipIndex != 0 || pl.Gain != -3 {
		t.Errorf("payload = %+v, want kick/0/-3", pl)
	}
	if got := s.Get().Tracks[0].Clips[0].Gain; got != -3 {
		t.Errorf("clip gain = %v, want -3", got)
	}

	// Out-of-range index: no event, no change, no panic.
	s.Apply(SetClipGain("kick", 99, -12))
	select {
	case ev := <-ch:
		t.Errorf("unexpected event %s for out-of-range clip index", ev.Type)
	default:
	}
}
