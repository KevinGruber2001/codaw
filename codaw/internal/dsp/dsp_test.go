package dsp

import (
	"math"
	"testing"

	"github.com/KevinGruber2001/codaw/internal/schema"
)

const rate = 48000

// Every effect the schema declares sounds like something, and nothing sounds that is not declared.
func TestEffectsMatchTheSchema(t *testing.T) {
	declared := map[string]bool{}
	for _, effect := range schema.Effects {
		declared[effect.Name] = true
		e, err := New(effect.Name, rate, nil)
		if err != nil {
			t.Errorf("%s is in the schema but has no implementation: %v", effect.Name, err)
			continue
		}
		for _, param := range effect.Params {
			if _, ok := schema.AsNumber(param.Default); !ok {
				t.Errorf("%s.%s has no default", effect.Name, param.Name)
			}
			if !e.SetParam(param.Name, 0) {
				t.Errorf("%s does not take %s, which the schema gives it", effect.Name, param.Name)
			}
		}
		if e.SetParam("no_such_parameter", 0) {
			t.Errorf("%s takes a parameter it does not have", effect.Name)
		}
	}
	for _, name := range Names() {
		if !declared[name] {
			t.Errorf("%s is implemented but not in the schema", name)
		}
	}
}

func TestSilenceInSilenceOut(t *testing.T) {
	for _, name := range Names() {
		e, _ := New(name, rate, nil)
		buf := make([]float32, 512*2)
		for block := 0; block < 20; block++ {
			e.Process(buf, 512)
		}
		for i, x := range buf {
			if x != 0 {
				t.Errorf("%s: sample %d is %v, not silent", name, i, x)
				break
			}
		}
	}
	l := NewLimiter(rate)
	buf := make([]float32, 512*2)
	l.Process(buf, 512)
	if rms(buf) != 0 {
		t.Error("limiter: not silent")
	}
}

// sine is frames of a sine at hz, the same on both channels.
func sine(hz, amplitude float64, frames int) []float32 {
	buf := make([]float32, frames*2)
	for f := 0; f < frames; f++ {
		x := float32(amplitude * math.Sin(2*math.Pi*hz*float64(f)/rate))
		buf[f*2], buf[f*2+1] = x, x
	}
	return buf
}

func rms(buf []float32) float64 {
	sum := 0.0
	for _, x := range buf {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum / float64(len(buf)))
}

// process runs a buffer through an effect a block at a time, and returns the second half, after the
// filters have settled.
func process(e Effect, buf []float32) []float32 {
	const block = 512
	for at := 0; at < len(buf)/2; at += block {
		n := min(block, len(buf)/2-at)
		e.Process(buf[at*2:(at+n)*2], n)
	}
	return buf[len(buf)/2:]
}

func TestEQBoostsItsBand(t *testing.T) {
	cases := []struct {
		param       string
		hz, otherHz float64
	}{
		{"low_db", 60, 5000},
		{"mid_db", 1000, 60},
		{"high_db", 12000, 200},
	}
	for _, c := range cases {
		gain := func(hz float64) float64 {
			e, _ := New("eq_3band", rate, map[string]any{c.param: 12.0})
			return Lin2DB(rms(process(e, sine(hz, 0.25, rate))) / (0.25 / math.Sqrt2))
		}
		if boosted, other := gain(c.hz), gain(c.otherHz); boosted < 9 || math.Abs(other) > 3 {
			t.Errorf("%s +12 dB: %.0f Hz comes out %+.1f dB, %.0f Hz %+.1f dB", c.param, c.hz, boosted, c.otherHz, other)
		}
	}
}

func TestCompressorTurnsDownLoudParts(t *testing.T) {
	e, _ := New("compressor", rate, map[string]any{"threshold": -20.0, "ratio": 4.0, "attack_ms": 1.0, "release_ms": 50.0})
	in := 0.5 // -6 dBFS peak: 14 dB over
	out := rms(process(e, sine(440, in, rate))) / (in / math.Sqrt2)
	// 14 dB over at 4:1 comes out 3.5 dB over: 10.5 dB turned down.
	if db := Lin2DB(out); db > -9.5 || db < -11.5 {
		t.Errorf("compressor took %.1f dB off, want about -10.5", db)
	}
	quiet, _ := New("compressor", rate, map[string]any{"threshold": -20.0})
	if db := Lin2DB(rms(process(quiet, sine(440, 0.05, rate))) / (0.05 / math.Sqrt2)); math.Abs(db) > 0.1 {
		t.Errorf("a signal under the threshold changed by %.2f dB", db)
	}
}

func TestLimiterNeverExceedsTheCeiling(t *testing.T) {
	l := NewLimiter(rate)
	buf := sine(100, 4, rate) // +12 dBFS
	for i := 0; i < len(buf); i += 2 * 300 {
		buf[i] = 8 // and some spikes
	}
	for at := 0; at < rate; at += 480 {
		l.Process(buf[at*2:(at+480)*2], 480)
	}
	for i, x := range buf {
		if math.Abs(float64(x)) > Ceiling+1e-6 {
			t.Fatalf("sample %d is %v, over the ceiling", i, x)
		}
	}
}

func TestReverbRingsOn(t *testing.T) {
	e, _ := New("reverb", rate, map[string]any{"wet": 0.5, "room_size": 0.8})
	buf := make([]float32, rate*2)
	copy(buf, sine(440, 0.5, rate/10))
	process(e, buf)
	if tail := rms(buf[len(buf)/2:]); tail < 1e-4 {
		t.Errorf("nothing left half a second after the sound stopped (rms %g)", tail)
	}
}

// A parameter that changes while playing glides: no block jumps by the whole change.
func TestParametersAreSmoothed(t *testing.T) {
	var s smoothed
	s.set(0)
	s.snap()
	s.set(1)
	if first := s.next(256, rate); first <= 0 || first >= 0.5 {
		t.Errorf("first block after a jump to 1 is at %v", first)
	}
	for i := 0; i < 200; i++ {
		s.next(256, rate)
	}
	if s.value != 1 {
		t.Errorf("never got there: %v", s.value)
	}
}

func TestProcessDoesNotAllocate(t *testing.T) {
	buf := sine(440, 0.5, 512)
	for _, name := range Names() {
		e, _ := New(name, rate, nil)
		e.SetParam(schemaParam(name), 0.3)
		if n := testing.AllocsPerRun(50, func() { e.Process(buf, 512) }); n != 0 {
			t.Errorf("%s allocates %v times per block", name, n)
		}
	}
}

func schemaParam(name string) string {
	effect, _ := schema.EffectNamed(name)
	return effect.Params[0].Name
}
