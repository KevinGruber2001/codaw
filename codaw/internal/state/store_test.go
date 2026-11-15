package state

import (
	"sync"
	"testing"

	"github.com/KevinGruber2001/codaw/internal/project"
)

func testProject(gain float64) *project.Project {
	return &project.Project{
		Transport: project.Transport{BPM: 120, SampleRate: 48000},
		Master:    &project.Master{},
		Tracks:    []*project.Track{{ID: "kick", Gain: gain}},
	}
}

// A snapshot taken before a Set still reads as it was.
func TestSnapshotsDoNotChange(t *testing.T) {
	s := New(testProject(0))
	before := s.Get()
	s.Set(testProject(-30))
	if before.Tracks[0].Gain != 0 || s.Get().Tracks[0].Gain != -30 {
		t.Errorf("before %v, after %v; want 0, -30", before.Tracks[0].Gain, s.Get().Tracks[0].Gain)
	}
}

// For the race detector: reading while another goroutine sets.
func TestConcurrentSetAndGet(t *testing.T) {
	s := New(testProject(0))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			next := s.Get().Clone()
			next.Tracks[0].Gain = float64(-i % 40)
			s.Set(next)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			p := s.Get()
			_ = p.Tracks[0].Gain + p.Transport.BPM
		}
	}()
	wg.Wait()
}

// A subscriber hears of every Set it has room for, and is never blocked on.
func TestSubscribers(t *testing.T) {
	s := New(testProject(0))
	ch := make(chan Event, 1)
	unsubscribe := s.Subscribe(ch)
	s.Set(testProject(-1))
	s.Set(testProject(-2)) // the channel is full: dropped, but the first one says there is news
	if got := (<-ch).Project.Tracks[0].Gain; got != -1 {
		t.Errorf("event for gain %v, want -1", got)
	}
	if got := s.Get().Tracks[0].Gain; got != -2 {
		t.Errorf("Get says %v, want the latest, -2", got)
	}
	unsubscribe()
	s.Set(testProject(-3))
	if len(ch) != 0 {
		t.Error("an event after unsubscribing")
	}
}
