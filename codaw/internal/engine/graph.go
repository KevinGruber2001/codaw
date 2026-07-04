package engine

import (
	"fmt"
	"path/filepath"

	"github.com/KevinGruber2001/codaw/internal/audio"
	"github.com/KevinGruber2001/codaw/internal/project"
)

// scheduledClip pairs a loaded Sound with the timeline info we need to place it
// when playback starts. We keep the beat positions (not pre-computed frames)
// because the frame offset depends on BPM, which can change at runtime.
type scheduledClip struct {
	sound     *audio.Sound
	startBeat float64
	endBeat   float64
}

// buildGraph constructs the whole audio routing tree from a project:
//
//	clip(Sound) → track(Group) → bus(Group) → master(Group) → speakers
//
// Each arrow is a parent attachment; miniaudio sums and mixes automatically as
// audio flows up the tree. We only set gains/pans and remember the clips so
// Play can schedule them. Nothing is audible yet — sounds aren't started here.
func (e *Engine) buildGraph(p *project.Project) error {
	// 1. Master group → its FX chain → engine endpoint (the speakers).
	master, err := e.audio.NewGroup(nil)
	if err != nil {
		return fmt.Errorf("engine: build master: %w", err)
	}
	master.SetGainDB(p.Master.Gain)
	e.master = master

	masterFX, err := e.buildChain(p.Master.FX)
	if err != nil {
		return fmt.Errorf("engine: build master fx: %w", err)
	}
	e.fx["master"] = masterFX
	// dst nil → the chain feeds the endpoint.
	if err := e.audio.AttachChain(master, attachable(masterFX), nil); err != nil {
		return fmt.Errorf("engine: wire master fx: %w", err)
	}

	// 2. Bus groups: bus → its FX chain → master.
	for _, b := range p.Buses {
		g, err := e.audio.NewGroup(master)
		if err != nil {
			return fmt.Errorf("engine: build bus %q: %w", b.ID, err)
		}
		g.SetGainDB(b.Gain)
		e.buses[b.ID] = g

		busFX, err := e.buildChain(b.FX)
		if err != nil {
			return fmt.Errorf("engine: build bus %q fx: %w", b.ID, err)
		}
		e.fx[b.ID] = busFX
		if err := e.audio.AttachChain(g, attachable(busFX), master); err != nil {
			return fmt.Errorf("engine: wire bus %q fx: %w", b.ID, err)
		}
	}

	// 3. Track groups, parented to their bus (or master if no bus), then the
	//    clips on each track as Sounds attached to the track group.
	for _, t := range p.Tracks {
		parent := master
		if t.Bus != "" {
			// The loader's validation guarantees this bus exists, but guard
			// anyway so a bad project can't nil-panic the audio thread.
			if b, ok := e.buses[t.Bus]; ok {
				parent = b
			} else {
				return fmt.Errorf("engine: track %q routes to unknown bus %q", t.ID, t.Bus)
			}
		}

		g, err := e.audio.NewGroup(parent)
		if err != nil {
			return fmt.Errorf("engine: build track %q: %w", t.ID, err)
		}
		e.tracks[t.ID] = g
		// gain + pan + mute/solo in one place (mixer.go).
		e.applyTrackMix(p, t)

		// track → its FX chain → parent (bus or master).
		trackFX, err := e.buildChain(t.FX)
		if err != nil {
			return fmt.Errorf("engine: build track %q fx: %w", t.ID, err)
		}
		e.fx[t.ID] = trackFX
		if err := e.audio.AttachChain(g, attachable(trackFX), parent); err != nil {
			return fmt.Errorf("engine: wire track %q fx: %w", t.ID, err)
		}

		for i, c := range t.Clips {
			full := filepath.Join(p.RootDir, c.File)
			s, err := e.audio.NewSound(full, g)
			if err != nil {
				return fmt.Errorf("engine: track %q clip[%d]: %w", t.ID, i, err)
			}
			s.SetGainDB(c.Gain)
			s.SetLooping(c.Loop)
			if c.Offset > 0 {
				if err := s.SeekToSecond(c.Offset); err != nil {
					return fmt.Errorf("engine: track %q clip[%d]: %w", t.ID, i, err)
				}
			}
			e.clips = append(e.clips, scheduledClip{
				sound:     s,
				startBeat: c.Start,
				endBeat:   c.End,
			})
		}
	}

	// Resolve automation curves now that all groups/effects exist.
	e.buildLanes(p)

	return nil
}

// teardownGraph closes every node in reverse order of creation: clips first,
// then track/bus groups, then master. Children must be freed before parents.
func (e *Engine) teardownGraph() {
	for _, sc := range e.clips {
		sc.sound.Close()
	}
	e.clips = nil
	e.lanes = nil

	// Effects sit between groups; uninit them after sounds, before groups.
	for id, chain := range e.fx {
		for _, ef := range chain {
			if ef != nil {
				ef.Close()
			}
		}
		delete(e.fx, id)
	}

	for id, g := range e.tracks {
		g.Close()
		delete(e.tracks, id)
	}
	for id, g := range e.buses {
		g.Close()
		delete(e.buses, id)
	}
	if e.master != nil {
		e.master.Close()
		e.master = nil
	}
}
