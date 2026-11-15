package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/state"
)

// The test project renders to a WAV as long as its last clip plus the tail, and it is not silent.
func TestRender(t *testing.T) {
	p, err := project.Load("../../testdata/basic/project.toml")
	if err != nil {
		t.Fatal(err)
	}
	e, _ := NewOffline(state.New(p))
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.wav")
	if err := e.Render(out, 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	lastBeat := 0.0
	for _, tr := range p.Tracks {
		for _, c := range tr.Clips {
			lastBeat = max(lastBeat, c.End)
		}
	}
	rate := p.Transport.SampleRate
	frames := int(lastBeat*float64(rate)*60/p.Transport.BPM) + rate
	if want := 44 + frames*4; len(data) != want {
		t.Errorf("%d bytes, want %d", len(data), want)
	}
	loud := 0
	for i := 44; i+1 < len(data); i += 2 {
		if v := int16(uint16(data[i]) | uint16(data[i+1])<<8); v > 100 || v < -100 {
			loud++
		}
	}
	if loud == 0 {
		t.Error("the render is silent")
	}
}
