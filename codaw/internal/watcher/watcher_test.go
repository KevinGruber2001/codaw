package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/schema"
	"github.com/KevinGruber2001/codaw/internal/state"
)

const trackText = `id = "k"
bus = "a"
gain = -3.0

[[fx]]
type = "reverb"
wet = 0.9

[[clip]]
file = "samples/x.wav"
start = 0.0
end = 4.0
loop = false
gain = -1.0

[[automation]]
target = "pan"

[[automation.point]]
beat = 0
value = -0.5
`

type fixture struct {
	t      *testing.T
	root   string
	store  *state.Store
	w      *Watcher
	events chan state.Event
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir(), events: make(chan state.Event, 64)}
	for name, text := range map[string]string{
		"project.toml": `[project]
name = "t"
[transport]
bpm = 120.0
time_sig = "4/4"
sample_rate = 48000
bit_depth = 24
[layout]
master = "master.toml"
buses = ["buses/a.toml", "buses/b.toml"]
tracks = ["tracks/k.toml"]
`,
		"master.toml":   "gain = 0.0\n",
		"buses/a.toml":  "id = \"a\"\n",
		"buses/b.toml":  "id = \"b\"\n",
		"tracks/k.toml": trackText,
		"samples/x.wav": "x",
	} {
		f.write(name, text)
	}
	p, err := project.Load(filepath.Join(f.root, "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := project.Validate(p); err != nil {
		t.Fatal(err)
	}
	f.store = state.New(p)
	f.store.Subscribe(f.events)
	if f.w, err = New(f.store); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) write(name, text string) {
	f.t.Helper()
	path := filepath.Join(f.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// save writes a file and lets the watcher handle it, as the event loop would. It returns how many
// events the store sent.
func (f *fixture) save(name, text string) int {
	f.t.Helper()
	f.write(name, text)
	f.w.handleChange(filepath.Join(f.root, filepath.FromSlash(name)))
	n := 0
	for len(f.events) > 0 {
		<-f.events
		n++
	}
	return n
}

func edit(t *testing.T, old, new string) string {
	t.Helper()
	if !strings.Contains(trackText, old) {
		t.Fatalf("the track has no %q", old)
	}
	return strings.Replace(trackText, old, new, 1)
}

// Every edit reaches the store: as a small mutation where the engine can change the value while it
// plays, as a replaced track otherwise. None is dropped.
// Every edit reaches the store, as one new project; saving without a change sends nothing.
func TestEveryEditIsApplied(t *testing.T) {
	for _, tt := range []struct {
		name  string
		text  string
		check func(*project.Track) bool
	}{
		{"gain", edit(t, "gain = -3.0", "gain = -9.0"), func(k *project.Track) bool { return k.Gain == -9 }},
		{"clip gain", edit(t, "gain = -1.0", "gain = -2.0"), func(k *project.Track) bool { return k.Clips[0].Gain == -2 }},
		{"effect parameter", edit(t, "wet = 0.9", "wet = 0.1"), func(k *project.Track) bool { return k.FX[0].Params["wet"] == 0.1 }},
		{"integer for a float", edit(t, "wet = 0.9", "wet = 1"), func(k *project.Track) bool { v, _ := schema.AsNumber(k.FX[0].Params["wet"]); return v == 1 }},
		{"effect parameter removed", edit(t, "wet = 0.9\n", ""), func(k *project.Track) bool { _, set := k.FX[0].Params["wet"]; return !set }},
		{"bus", edit(t, `bus = "a"`, `bus = "b"`), func(k *project.Track) bool { return k.Bus == "b" }},
		{"loop", edit(t, "loop = false", "loop = true"), func(k *project.Track) bool { return k.Clips[0].Loop }},
		{"clip moved", edit(t, "end = 4.0", "end = 8.0"), func(k *project.Track) bool { return k.Clips[0].End == 8 }},
		{"automation point", edit(t, "value = -0.5", "value = 0.5"), func(k *project.Track) bool { return k.Automation[0].Points[0].Value == 0.5 }},
		{"automation target", edit(t, `target = "pan"`, `target = "gain"`), func(k *project.Track) bool { return k.Automation[0].Target == "gain" }},
		{"automation removed", trackText[:strings.Index(trackText, "[[automation]]")], func(k *project.Track) bool { return len(k.Automation) == 0 }},
		{"renamed", edit(t, `id = "k"`, `id = "kick"`), func(k *project.Track) bool { return k.ID == "kick" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if n := f.save("tracks/k.toml", tt.text); n != 1 {
				t.Errorf("%d events, want 1", n)
			}
			if !tt.check(f.store.Get().Tracks[0]) {
				t.Errorf("the store does not have the change: %+v", *f.store.Get().Tracks[0])
			}
		})
	}
	f := newFixture(t)
	if n := f.save("tracks/k.toml", trackText); n != 0 {
		t.Errorf("saved unchanged: %d events, want none", n)
	}
	// Again from a clone in the store, which must compare equal to its file too.
	f.store.Set(f.store.Get().Clone())
	<-f.events
	if n := f.save("tracks/k.toml", trackText); n != 0 {
		t.Errorf("saved unchanged over a clone: %d events, want none", n)
	}
}

func TestBusAndMasterEdits(t *testing.T) {
	f := newFixture(t)
	if n := f.save("buses/b.toml", "id = \"b\"\ngain = -4.0\n"); n != 1 {
		t.Errorf("bus gain: %d events", n)
	}
	if n := f.save("buses/b.toml", "id = \"b\"\ngain = -4.0\n\n[[fx]]\ntype = \"reverb\"\n"); n != 1 {
		t.Errorf("bus effect added: %d events", n)
	}
	if n := f.save("master.toml", "gain = 0.0\nlimiter = true\n"); n != 1 {
		t.Errorf("limiter: %d events", n)
	}
	if !f.store.Get().Master.Limiter || len(f.store.Get().Buses[1].FX) != 1 || f.store.Get().Buses[1].Gain != -4 {
		t.Error("the store does not have the changes")
	}
}

// An invalid file is not applied, and the engine keeps the last valid version.
func TestInvalidEditIsRejected(t *testing.T) {
	f := newFixture(t)
	if n := f.save("tracks/k.toml", edit(t, "gain = -3.0", "gain = 50.0")); n != 0 {
		t.Errorf("%d events, want none", n)
	}
	if gain := f.store.Get().Tracks[0].Gain; gain != -3 {
		t.Errorf("gain %v, want the last valid -3", gain)
	}
	if n := f.save("tracks/k.toml", edit(t, "gain = -3.0", "gain = -5.0")); n != 1 {
		t.Errorf("after the fix: %d events", n)
	}
}

// Renaming a bus takes two saves, neither valid without the other. The first waits for the second.
func TestRenameAcrossTwoFiles(t *testing.T) {
	f := newFixture(t)
	if n := f.save("buses/a.toml", "id = \"drums\"\n"); n != 0 {
		t.Errorf("the bus alone: %d events, want none", n)
	}
	f.save("tracks/k.toml", edit(t, `bus = "a"`, `bus = "drums"`))
	p := f.store.Get()
	if p.Buses[0].ID != "drums" || p.Tracks[0].Bus != "drums" {
		t.Errorf("bus %q, track sends to %q; want drums for both", p.Buses[0].ID, p.Tracks[0].Bus)
	}
	if err := project.Validate(p); err != nil {
		t.Errorf("the store holds an invalid project: %v", err)
	}
}

// Run with -race: the loop and its debounce timer used to share a map between two goroutines.
func TestSavesWhileRunning(t *testing.T) {
	f := newFixture(t)
	if err := f.w.Start(); err != nil {
		t.Fatal(err)
	}
	defer f.w.Stop()
	for i := 0; i < 30; i++ {
		f.write("tracks/k.toml", edit(t, "gain = -3.0", fmt.Sprintf("gain = -%d.0", i%20+1)))
		f.write("buses/a.toml", fmt.Sprintf("id = \"a\"\ngain = -%d.0\n", i%20))
		time.Sleep(time.Duration(30+i%4*15) * time.Millisecond)
	}
	deadline := time.Now().Add(2 * time.Second)
	for f.store.Get().Tracks[0].Gain != -10 || f.store.Get().Buses[0].Gain != -9 {
		if time.Now().After(deadline) {
			t.Fatalf("the last save did not arrive: track %v dB, bus %v dB", f.store.Get().Tracks[0].Gain, f.store.Get().Buses[0].Gain)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Every key the schema has for a track reaches the store when it changes. A key added to the schema
// fails here until it has a value to change to.
func TestEveryKeyReachesTheStore(t *testing.T) {
	changes := map[string][2]string{ // section.key → the line in trackText, and what to change it to
		".id":                    {`id = "k"`, `id = "kick"`},
		".bus":                   {`bus = "a"`, `bus = "b"`},
		".gain":                  {"gain = -3.0", "gain = -9.0"},
		".pan":                   {"gain = -3.0", "gain = -3.0\npan = 0.5"},
		".mute":                  {"gain = -3.0", "gain = -3.0\nmute = true"},
		".solo":                  {"gain = -3.0", "gain = -3.0\nsolo = true"},
		"clip.file":              {"samples/x.wav", "samples/y.wav"},
		"clip.start":             {"start = 0.0", "start = 1.0"},
		"clip.end":               {"end = 4.0", "end = 8.0"},
		"clip.offset":            {"loop = false", "loop = false\noffset = 0.1"},
		"clip.loop":              {"loop = false", "loop = true"},
		"clip.gain":              {"gain = -1.0", "gain = -2.0"},
		"fx.type":                {`type = "reverb"`, `type = "eq_3band"`},
		"automation.target":      {`target = "pan"`, `target = "gain"`},
		"automation.point.beat":  {"beat = 0", "beat = 2"},
		"automation.point.value": {"value = -0.5", "value = 0.5"},
	}
	track, _ := schema.FileOf(schema.TrackFile)
	for _, section := range track.Sections {
		for _, key := range section.Keys {
			name := section.Name + "." + key.Name
			change, ok := changes[name]
			if !ok {
				t.Errorf("%s: no change to try; add one to this test", name)
				continue
			}
			f := newFixture(t)
			f.write("samples/y.wav", "y")
			before := f.store.Get()
			if n := f.save("tracks/k.toml", edit(t, change[0], change[1])); n != 1 {
				t.Errorf("%s: %d events, want one", name, n)
			}
			if f.store.Get() == before {
				t.Errorf("%s: the store still has the old project", name)
			}
		}
	}
}
