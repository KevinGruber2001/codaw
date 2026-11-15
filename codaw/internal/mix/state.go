package mix

import (
	"log"
	"path/filepath"

	"github.com/KevinGruber2001/codaw/internal/dsp"
	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/schema"
)

// trackState is what the mixer keeps for a track: what it read from the snapshot, ready to mix, and
// what has to outlive snapshots (effects, gain ramps).
type trackState struct {
	id      string
	clips   []clipState
	fx      []fxSlot
	lanes   []lane
	bus     *busState // nil: the master
	audible bool      // mute and solo, worked out across all tracks

	// The values this block mixes at: the file's, unless a lane automates them.
	gain, pan   float64
	left, right level
}

type busState struct {
	id    string
	buf   []float32
	fx    []fxSlot
	gain  float64
	level level
}

type masterState struct {
	fx      []fxSlot
	gain    float64
	limiter *dsp.Limiter
	level   level
}

// fxSlot is an effect at a position of a chain. It is kept across snapshots as long as that position
// holds an effect of the same type: then only its parameters change.
type fxSlot struct {
	fxType string
	effect dsp.Effect
}

// sync takes up a new snapshot. It reuses what it can of the old one, by ID: a track's effects keep
// their state, its gain ramps on from where it was. Runs on the mixing goroutine, between blocks.
func (m *Mixer) sync(p *project.Project) {
	old := m.proj
	m.proj = p
	if p == nil {
		return
	}

	// A new tempo keeps the playhead on the same beat, not the same frame.
	framesPer := float64(m.rate) * 60 / p.Transport.BPM
	if m.framesPer > 0 && framesPer != m.framesPer {
		m.pos = uint64(float64(m.pos) / m.framesPer * framesPer)
	}
	m.framesPer = framesPer

	oldBuses := m.busByID
	m.busByID = make(map[string]*busState, len(p.Buses))
	m.buses = m.buses[:0]
	for _, b := range p.Buses {
		st := oldBuses[b.ID]
		if st == nil {
			st = &busState{id: b.ID, buf: make([]float32, maxBlock*Channels)}
		}
		st.gain = b.Gain
		st.fx = m.chain(st.fx, b.FX)
		m.busByID[b.ID] = st
		m.buses = append(m.buses, st)
	}

	m.master.gain = p.Master.Gain
	m.master.fx = m.chain(m.master.fx, p.Master.FX)
	switch {
	case p.Master.Limiter && m.master.limiter == nil:
		m.master.limiter = dsp.NewLimiter(m.rate)
	case !p.Master.Limiter:
		m.master.limiter = nil
	}

	oldTracks := make(map[string]*trackState, len(m.tracks))
	for _, t := range m.tracks {
		oldTracks[t.id] = t
	}
	anySolo := false
	for _, t := range p.Tracks {
		anySolo = anySolo || t.Solo
	}
	m.tracks = m.tracks[:0]
	for _, t := range p.Tracks {
		st := oldTracks[t.ID]
		if st == nil {
			st = &trackState{id: t.ID}
		}
		st.gain, st.pan = t.Gain, t.Pan
		st.audible = !t.Mute && (!anySolo || t.Solo)
		st.bus = m.busByID[t.Bus]
		st.fx = m.chain(st.fx, t.FX)
		st.clips = m.clips(p, t)
		st.lanes = lanesOf(t)
		m.tracks = append(m.tracks, st)
	}
	if old == nil {
		log.Printf("[mix] %d track(s), %d bus(es), %.0f BPM, %d Hz", len(p.Tracks), len(p.Buses), p.Transport.BPM, m.rate)
	}
}

// chain brings an effect chain in line with the snapshot's: an effect keeps its state where the
// same type is still at its position; anything else is made new.
func (m *Mixer) chain(old []fxSlot, fxs []project.FX) []fxSlot {
	out := make([]fxSlot, 0, len(fxs))
	for i, fx := range fxs {
		if i < len(old) && old[i].fxType == fx.Type {
			slot := old[i]
			if declared, ok := schema.EffectNamed(fx.Type); ok {
				for name, value := range declared.Resolve(fx.Params) {
					slot.effect.SetParam(name, value)
				}
			}
			out = append(out, slot)
			continue
		}
		effect, err := dsp.New(fx.Type, m.rate, fx.Params)
		if err != nil {
			log.Printf("[mix] %v: skipping it", err) // validation rejects these, so only a bug gets here
			continue
		}
		out = append(out, fxSlot{fxType: fx.Type, effect: effect})
	}
	return out
}

// clips times a track's clips in frames and finds their audio.
func (m *Mixer) clips(p *project.Project, t *project.Track) []clipState {
	out := make([]clipState, 0, len(t.Clips))
	for i, c := range t.Clips {
		buf, err := m.cache.Load(filepath.Join(p.RootDir, c.File))
		if err != nil {
			log.Printf("[mix] track %s clip %d: %v (it stays silent)", t.ID, i, err)
			continue
		}
		out = append(out, clipState{
			buf:    buf,
			start:  uint64(c.Start * m.framesPer),
			end:    uint64(c.End * m.framesPer),
			offset: int(c.Offset * float64(m.rate)),
			loop:   c.Loop,
			gain:   float32(dsp.DB2Lin(c.Gain)),
		})
	}
	return out
}

// Preload decodes every sample a project uses into the cache, so the mixing goroutine finds them
// there when it takes the project up. The first error is returned; the rest are tried anyway.
func Preload(p *project.Project, load func(path string) error) error {
	var first error
	for _, t := range p.Tracks {
		for _, c := range t.Clips {
			if err := load(filepath.Join(p.RootDir, c.File)); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}
