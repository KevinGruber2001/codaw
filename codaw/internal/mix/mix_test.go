package mix

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/sample"
	"github.com/KevinGruber2001/codaw/internal/wav"
)

const rate = 48000

// At 120 BPM a beat is half a second: 24000 frames.
const beat = rate / 2

// writeWAV writes a mono file of the given samples and returns its name in dir.
func writeWAV(t *testing.T, dir, name string, samples []float32) string {
	t.Helper()
	w, err := wav.Create(filepath.Join(dir, name), rate, 2)
	if err != nil {
		t.Fatal(err)
	}
	stereo := make([]float32, len(samples)*2)
	for i, x := range samples {
		stereo[i*2], stereo[i*2+1] = x, x
	}
	if err := w.WriteFloat32(stereo); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func constant(v float32, frames int) []float32 {
	s := make([]float32, frames)
	for i := range s {
		s[i] = v
	}
	return s
}

func newProject(dir string, tracks ...*project.Track) *project.Project {
	return &project.Project{
		Transport: project.Transport{BPM: 120, SampleRate: rate},
		Master:    &project.Master{},
		Tracks:    tracks,
		RootDir:   dir,
	}
}

func newMixer(p *project.Project) *Mixer {
	m := New(rate, sample.NewCache(rate))
	m.Publish(p)
	return m
}

// render plays frames from the start and returns them.
func render(m *Mixer, frames int) []float32 {
	m.Play()
	out := make([]float32, frames*2)
	for at := 0; at < frames; at += 512 {
		n := min(512, frames-at)
		m.MixBlock(out[at*2 : (at+n)*2])
	}
	return out
}

func TestSilenceInSilenceOut(t *testing.T) {
	m := newMixer(newProject(t.TempDir(), &project.Track{ID: "t"}))
	for i, x := range render(m, rate) {
		if x != 0 {
			t.Fatalf("sample %d is %v", i, x)
		}
	}
}

// A track at -6 dB, panned to the centre, comes out at half the clip's level, times the -3 dB of the
// pan law.
func TestTrackGain(t *testing.T) {
	dir := t.TempDir()
	file := writeWAV(t, dir, "one.wav", constant(0.5, rate))
	p := newProject(dir, &project.Track{ID: "t", Gain: -6.0206, Clips: []project.Clip{{File: file, End: 1}}})
	out := render(newMixer(p), beat)
	got := out[beat] // mid-clip, left channel
	want := 0.5 * 0.5 * math.Sqrt(0.5)
	if math.Abs(float64(got)-want) > 1e-3 {
		t.Errorf("left = %v, want %v", got, want)
	}
}

func TestHardLeftPan(t *testing.T) {
	dir := t.TempDir()
	file := writeWAV(t, dir, "one.wav", constant(0.5, rate))
	p := newProject(dir, &project.Track{ID: "t", Pan: -1, Clips: []project.Clip{{File: file, End: 1}}})
	out := render(newMixer(p), beat)
	for f := 0; f < beat; f++ {
		if out[f*2+1] != 0 {
			t.Fatalf("right channel has %v at frame %d", out[f*2+1], f)
		}
	}
	if out[beat/2*2] < 0.49 {
		t.Errorf("left channel only has %v", out[beat/2*2])
	}
}

// A looping clip plays the file from the offset to its end, then from its start again.
func TestLoopWraps(t *testing.T) {
	dir := t.TempDir()
	ramp := make([]float32, 1000)
	for i := range ramp {
		ramp[i] = float32(i) / 2000 // a ramp, so each frame says where in the file it came from
	}
	file := writeWAV(t, dir, "ramp.wav", ramp)
	p := newProject(dir, &project.Track{ID: "t", Pan: -1, Clips: []project.Clip{{
		File: file, End: 1, Loop: true, Offset: 600.0 / rate,
	}}})
	out := render(newMixer(p), beat)
	at := func(frame int) float64 { return float64(out[frame*2]) }
	// Frame 300 into the clip is file frame 900; frame 400 has wrapped round to file frame 0... 100
	// frames into the second pass: file frame 0 is at clip frame 400, 50 at 450.
	for _, c := range []struct{ frame, file int }{{300, 900}, {450, 50}, {1450, 50}} {
		if got, want := at(c.frame), float64(c.file)/2000; math.Abs(got-want) > 1e-3 {
			t.Errorf("clip frame %d: %v, want file frame %d (%v)", c.frame, got, c.file, want)
		}
	}
}

// Clip edges fade over 5 ms: no step at the start or the end.
func TestClipEdgesFade(t *testing.T) {
	dir := t.TempDir()
	file := writeWAV(t, dir, "one.wav", constant(0.5, rate))
	p := newProject(dir, &project.Track{ID: "t", Pan: -1, Clips: []project.Clip{{File: file, Start: 1, End: 2}}})
	out := render(newMixer(p), 3*beat)
	maxStep := 0.0
	for f := 1; f < 3*beat; f++ {
		maxStep = math.Max(maxStep, math.Abs(float64(out[f*2]-out[(f-1)*2])))
	}
	if maxStep > 0.5/200 {
		t.Errorf("the biggest step between two frames is %v", maxStep)
	}
}

// Gain automation is sampled from the playhead: at beat 1 of a lane from -60 to 0 dB over 2 beats,
// the track is at -30 dB.
func TestAutomation(t *testing.T) {
	dir := t.TempDir()
	file := writeWAV(t, dir, "one.wav", constant(0.5, 3*rate))
	p := newProject(dir, &project.Track{ID: "t", Pan: -1, Clips: []project.Clip{{File: file, End: 3}},
		Automation: []project.Automation{{Target: "gain", Points: []project.AutomationPoint{{Beat: 0, Value: -60}, {Beat: 2, Value: 0}}}}})
	out := render(newMixer(p), 2*beat)
	// Sampled once per block: the block holding beat 1 starts up to 512 frames (about 0.6 dB here)
	// before it.
	if got := 20 * math.Log10(float64(out[beat*2])/0.5); math.Abs(got+30) > 1 {
		t.Errorf("at beat 1: %.1f dB, want about -30", got)
	}
}

// A new snapshot keeps effect state where the chain did not change shape.
func TestEffectsSurviveAReload(t *testing.T) {
	dir := t.TempDir()
	reverb := func(wet float64) []project.FX {
		return []project.FX{{Type: "reverb", Params: map[string]any{"wet": wet}}}
	}
	p := newProject(dir, &project.Track{ID: "t", FX: reverb(0.3)})
	m := newMixer(p)
	render(m, 512)
	before := m.tracks[0].fx[0].effect

	p2 := p.Clone()
	p2.Tracks[0].FX = reverb(0.6)
	m.Publish(p2)
	render(m, 512)
	if m.tracks[0].fx[0].effect != before {
		t.Error("a changed parameter made a new effect")
	}

	p3 := p2.Clone()
	p3.Tracks[0].FX = []project.FX{{Type: "eq_3band"}}
	m.Publish(p3)
	render(m, 512)
	if m.tracks[0].fx[0].fxType != "eq_3band" {
		t.Error("a changed type kept the old effect")
	}
}

func TestMuteAndSolo(t *testing.T) {
	dir := t.TempDir()
	file := writeWAV(t, dir, "one.wav", constant(0.5, rate))
	clip := []project.Clip{{File: file, End: 1}}
	for _, c := range []struct {
		name         string
		a, b         project.Track
		wantA, wantB bool
	}{
		{"mute", project.Track{Mute: true}, project.Track{}, false, true},
		{"solo", project.Track{Solo: true}, project.Track{}, true, false},
		{"solo beats nothing muted", project.Track{Solo: true, Mute: true}, project.Track{}, false, false},
	} {
		a, b := c.a, c.b
		a.ID, a.Pan, a.Clips = "a", -1, clip
		b.ID, b.Pan, b.Clips = "b", 1, clip
		out := render(newMixer(newProject(dir, &a, &b)), beat)
		if gotA, gotB := out[beat] > 1e-6, out[beat+1] > 1e-6; gotA != c.wantA || gotB != c.wantB {
			t.Errorf("%s: a heard %v, b heard %v; want %v, %v", c.name, gotA, gotB, c.wantA, c.wantB)
		}
	}
}

// Stopping fades out over a block and holds the playhead; playing again carries on from there.
func TestStopHoldsThePlayhead(t *testing.T) {
	dir := t.TempDir()
	file := writeWAV(t, dir, "one.wav", constant(0.5, 4*rate))
	m := newMixer(newProject(dir, &project.Track{ID: "t", Clips: []project.Clip{{File: file, End: 8}}}))
	render(m, beat)
	m.Stop()
	out := make([]float32, 512*2)
	m.MixBlock(out) // the fade-out
	held := m.Position()
	m.MixBlock(out)
	if m.Position() != held {
		t.Errorf("the playhead moved while stopped: %v → %v", held, m.Position())
	}
	if out[0] != 0 {
		t.Errorf("still sound after the fade-out: %v", out[0])
	}
	m.Seek(4)
	m.MixBlock(out)
	if m.Position() != 4 {
		t.Errorf("seek to 4 put the playhead at %v", m.Position())
	}
}

func TestBPMChangeKeepsTheBeat(t *testing.T) {
	m := newMixer(newProject(t.TempDir(), &project.Track{ID: "t"}))
	render(m, 2*beat)
	p := newProject(m.proj.RootDir, &project.Track{ID: "t"})
	p.Transport.BPM = 60
	m.Publish(p)
	out := make([]float32, 2)
	m.Stop()
	m.MixBlock(out[:0])
	m.Play()
	m.MixBlock(out)
	if got := m.Position(); math.Abs(got-2) > 0.001 {
		t.Errorf("after a tempo change the playhead is at beat %v, want 2", got)
	}
}

func TestMixBlockDoesNotAllocate(t *testing.T) {
	dir := t.TempDir()
	file := writeWAV(t, dir, "one.wav", constant(0.5, rate))
	p := newProject(dir,
		&project.Track{ID: "a", Bus: "b", Clips: []project.Clip{{File: file, End: 4, Loop: true}},
			FX:         []project.FX{{Type: "eq_3band"}, {Type: "compressor"}, {Type: "reverb"}},
			Automation: []project.Automation{{Target: "fx[0].low_db", Points: []project.AutomationPoint{{Beat: 0, Value: 0}, {Beat: 4, Value: 6}}}}},
	)
	p.Buses = []*project.Bus{{ID: "b", FX: []project.FX{{Type: "compressor"}}}}
	p.Master.Limiter = true
	m := newMixer(p)
	out := make([]float32, 512*2)
	m.Play()
	m.MixBlock(out) // takes the snapshot up
	if n := testing.AllocsPerRun(100, func() { m.MixBlock(out) }); n != 0 {
		t.Errorf("MixBlock allocates %v times", n)
	}
}

func TestParseTarget(t *testing.T) {
	for _, c := range []struct {
		in    string
		ok    bool
		index int
		param string
	}{
		{"gain", true, 0, ""}, {"pan", true, 0, ""}, {"fx[2].wet", true, 2, "wet"},
		{"fx[x].wet", false, 0, ""}, {"fx[1]", false, 0, ""}, {"volume", false, 0, ""},
	} {
		_, index, param, err := parseTarget(c.in)
		if (err == nil) != c.ok || index != c.index || param != c.param {
			t.Errorf("%s: %d %q %v", c.in, index, param, err)
		}
	}
}
