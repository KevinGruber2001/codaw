// Package mix turns a project into audio, one block at a time.
//
// MixBlock is the whole signal path, readable top to bottom: clips into their track, the track's
// effects, its gain and pan, into its bus or the master; each bus's effects and gain into the master;
// the master's effects, gain and limiter, out. Playback and rendering both call it, so what you hear
// and what you export cannot drift apart.
//
// The mixer reads an immutable project snapshot. A new one (a saved TOML file) is handed over with
// Publish and taken up at the start of the next block; nothing has to be told what changed. What
// lives longer than a snapshot (effect state, where a fade currently is) is kept in the mixer, keyed
// by track or bus ID and effect position, so a save never resets a reverb tail or clicks a filter.
package mix

import (
	"math"
	"sync/atomic"

	"github.com/KevinGruber2001/codaw/internal/dsp"
	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/sample"
)

// Channels is how many channels the mixer produces: stereo, interleaved.
const Channels = 2

// maxBlock is the most frames MixBlock processes in one go; a longer buffer is mixed in parts. It
// sizes the scratch buffers, which are allocated once.
const maxBlock = 2048

// fadeTime is the fade at each edge of a clip. Not a nicety: a clip that starts or ends mid-waveform
// is a step in the signal, which is a click, and trimming and splitting make every edge a cut.
const fadeTime = 0.005 // seconds

// Mixer renders a project. One goroutine calls MixBlock; the transport methods (Play, Stop, Seek)
// and Publish may be called from any goroutine.
type Mixer struct {
	rate  int
	cache *sample.Cache

	// Handed over by other goroutines, taken up at the start of a block.
	next     atomic.Pointer[project.Project]
	playing  atomic.Bool
	seek     atomic.Pointer[float64]
	position atomic.Uint64 // the playhead after the last block, in beats, as math.Float64bits

	// Everything below belongs to the goroutine calling MixBlock.
	proj      *project.Project
	framesPer float64 // frames per beat
	pos       uint64  // the playhead, in frames: the one clock everything is timed by
	fade      int     // fadeTime in frames
	running   float32 // 0 when stopped, 1 when playing, in between while starting or stopping

	tracks  []*trackState // as the project lists them
	buses   []*busState
	busByID map[string]*busState
	master  masterState

	trackBuf  []float32
	masterBuf []float32
}

// New makes a mixer that renders at rate, decoding samples through cache (which must be at the same
// rate).
func New(rate int, cache *sample.Cache) *Mixer {
	return &Mixer{
		rate:      rate,
		cache:     cache,
		fade:      int(fadeTime * float64(rate)),
		busByID:   map[string]*busState{},
		trackBuf:  make([]float32, maxBlock*Channels),
		masterBuf: make([]float32, maxBlock*Channels),
	}
}

// Rate is the sample rate the mixer renders at.
func (m *Mixer) Rate() int { return m.rate }

// Publish hands over a new project snapshot, which the next block plays. Its samples should be in the
// cache already (see Preload), or the mixing goroutine decodes them itself.
func (m *Mixer) Publish(p *project.Project) { m.next.Store(p) }

// Play starts the transport where the playhead is.
func (m *Mixer) Play() { m.playing.Store(true) }

// Stop stops it, leaving the playhead where it is. Sound fades out over one block, not abruptly.
func (m *Mixer) Stop() { m.playing.Store(false) }

// Playing says whether the transport runs.
func (m *Mixer) Playing() bool { return m.playing.Load() }

// Seek moves the playhead to a beat, from the next block on.
func (m *Mixer) Seek(beat float64) {
	beat = math.Max(0, beat)
	m.seek.Store(&beat)
}

// Position is the playhead in beats, as of the end of the last block mixed.
func (m *Mixer) Position() float64 { return math.Float64frombits(m.position.Load()) }

// MixBlock fills out (interleaved stereo) with the next len(out)/2 frames, and moves the playhead on
// by as many while the transport runs. Never allocates, except to take up a new snapshot.
func (m *Mixer) MixBlock(out []float32) {
	for len(out) > 0 {
		n := min(len(out), maxBlock*Channels)
		m.mix(out[:n])
		out = out[n:]
	}
}

func (m *Mixer) mix(out []float32) {
	frames := len(out) / Channels
	if p := m.next.Load(); p != m.proj {
		m.sync(p)
	}
	if beat := m.seek.Swap(nil); beat != nil && m.framesPer > 0 {
		m.pos = uint64(*beat * m.framesPer)
	}
	if m.proj == nil {
		clear(out)
		return
	}

	// Starting or stopping ramps the clips in or out over this block. A stopped transport still runs
	// the effects, so a reverb rings out after Stop the way it does in a room.
	from := m.running
	to := float32(0)
	if m.playing.Load() {
		to = 1
	}
	m.running = to
	reading := from > 0 || to > 0

	// Automation is sampled once per block, at its start, from the frame clock.
	beat := float64(m.pos) / m.framesPer

	master := m.masterBuf[:len(out)]
	clear(master)
	for _, b := range m.buses {
		clear(b.buf[:len(out)])
	}

	// 1. Each track: its clips, its effects, its gain and pan, into its bus or the master.
	for _, t := range m.tracks {
		buf := m.trackBuf[:len(out)]
		clear(buf)
		if reading {
			for i := range t.clips {
				t.clips[i].mixInto(buf, m.pos, frames, m.fade)
			}
			if from != to {
				fadeTransport(buf, frames, m.fade, from, to)
			}
		}
		t.automate(beat)
		for _, fx := range t.fx {
			fx.effect.Process(buf, frames)
		}

		gain := t.gain
		if !t.audible {
			gain = math.Inf(-1)
		}
		l, r := panGains(t.pan)
		g := float32(dsp.DB2Lin(gain))
		t.left.to(g * l)
		t.right.to(g * r)
		dst := master
		if t.bus != nil {
			dst = t.bus.buf[:len(out)]
		}
		addRamped(dst, buf, frames, t.left.from, t.right.from, t.left.now, t.right.now)
	}

	// 2. Each bus: its effects and gain, into the master.
	for _, b := range m.buses {
		buf := b.buf[:len(out)]
		for _, fx := range b.fx {
			fx.effect.Process(buf, frames)
		}
		b.level.to(float32(dsp.DB2Lin(b.gain)))
		addRamped(master, buf, frames, b.level.from, b.level.from, b.level.now, b.level.now)
	}

	// 3. The master: its effects, its gain, the limiter, out.
	for _, fx := range m.master.fx {
		fx.effect.Process(master, frames)
	}
	m.master.level.to(float32(dsp.DB2Lin(m.master.gain)))
	copy(out, master)
	ramp(out, frames, m.master.level.from, m.master.level.from, m.master.level.now, m.master.level.now)
	if m.master.limiter != nil {
		m.master.limiter.Process(out, frames)
	}

	// The fade-out block after Stop reads on from the playhead without moving it, so Play carries
	// on from where the sound stopped.
	if to > 0 {
		m.pos += uint64(frames)
	}
	m.position.Store(math.Float64bits(float64(m.pos) / m.framesPer))
}

// level is a gain that changes over a block rather than at its start: from what the last block
// ended on to what this one should end on.
type level struct {
	from, now float32
	set       bool
}

// to sets the level this block ends on. The first time, there is nothing to ramp from.
func (l *level) to(v float32) {
	if !l.set {
		l.now, l.set = v, true
	}
	l.from, l.now = l.now, v
}

// ramp multiplies a block by a gain that moves in a straight line from (l0, r0) at its first frame
// to (l1, r1) at its last.
func ramp(buf []float32, frames int, l0, r0, l1, r1 float32) {
	if l0 == l1 && r0 == r1 {
		if l0 == 1 && r0 == 1 {
			return
		}
		for f := 0; f < frames; f++ {
			buf[f*2] *= l0
			buf[f*2+1] *= r0
		}
		return
	}
	dl, dr := (l1-l0)/float32(frames), (r1-r0)/float32(frames)
	for f := 0; f < frames; f++ {
		k := float32(f + 1)
		buf[f*2] *= l0 + dl*k
		buf[f*2+1] *= r0 + dr*k
	}
}

// fadeTransport fades a block in or out over its first `over` frames: what starting or stopping the
// transport does to the clips, so neither clicks. Past the fade the block is at `to`.
func fadeTransport(buf []float32, frames, over int, from, to float32) {
	over = max(1, min(over, frames))
	step := (to - from) / float32(over)
	for f := 0; f < frames; f++ {
		g := to
		if f < over {
			g = from + step*float32(f+1)
		}
		buf[f*2] *= g
		buf[f*2+1] *= g
	}
}

// addRamped adds src into dst under a ramped gain, as ramp does.
func addRamped(dst, src []float32, frames int, l0, r0, l1, r1 float32) {
	dl, dr := (l1-l0)/float32(frames), (r1-r0)/float32(frames)
	for f := 0; f < frames; f++ {
		k := float32(f + 1)
		dst[f*2] += src[f*2] * (l0 + dl*k)
		dst[f*2+1] += src[f*2+1] * (r0 + dr*k)
	}
}

// panGains turns a pan position (-1 left .. 1 right) into left and right factors with a constant
// power law: the centre is -3 dB on both sides, so a sound panned across the field does not get louder
// in the middle. Chosen once: changing it later would change how every project sounds.
func panGains(pan float64) (l, r float32) {
	pan = math.Max(-1, math.Min(1, pan))
	angle := (pan + 1) * math.Pi / 4
	return float32(math.Cos(angle)), float32(math.Sin(angle))
}
