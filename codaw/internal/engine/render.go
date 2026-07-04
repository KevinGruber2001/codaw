package engine

import (
	"fmt"
	"log"

	"github.com/KevinGruber2001/codaw/internal/audio"
	"github.com/KevinGruber2001/codaw/internal/state"
	"github.com/KevinGruber2001/codaw/internal/wav"
)

// NewOffline creates an engine with no audio device, for offline export. It
// builds the very same graph as a live engine (gain, pan, routing, EQ, reverb),
// but you drive it with Render instead of Play. It does not subscribe to the
// Store — there's no hot-reload during a render.
func NewOffline(store *state.Store) (*Engine, error) {
	p := store.Get()
	// Offline we honour the *project's* sample rate exactly (no device to
	// reconcile against). Stereo output.
	a, err := audio.NewOfflineEngine(uint32(p.Transport.SampleRate), 2)
	if err != nil {
		return nil, err
	}
	return &Engine{
		audio:  a,
		store:  store,
		buses:  make(map[string]*audio.Group),
		tracks: make(map[string]*audio.Group),
		fx:     make(map[string][]*audio.Effect),
		// events/unsubscribe stay nil — Close skips the subscription teardown.
	}, nil
}

// Render plays the whole project offline and writes it to a 16-bit WAV at
// outPath. tailSeconds is extra time rendered after the last clip ends so
// effect tails (e.g. the reverb echo) can ring out instead of being cut off.
//
// Call Load first to build the graph.
func (e *Engine) Render(outPath string, tailSeconds float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Total length = end of the last clip, plus the tail.
	var lastBeat float64
	for _, sc := range e.clips {
		if sc.endBeat > lastBeat {
			lastBeat = sc.endBeat
		}
	}
	ch := e.audio.Channels()
	totalFrames := e.transport.beatsToFrames(lastBeat) +
		uint64(tailSeconds*float64(e.transport.sampleRate))
	if totalFrames == 0 {
		return fmt.Errorf("engine: nothing to render (no clips)")
	}

	// Schedule every clip from the start of the timeline (no lead-in needed
	// offline — the clock doesn't move until we read).
	for _, sc := range e.clips {
		sc.sound.ScheduleStart(e.transport.beatsToFrames(sc.startBeat))
		if sc.endBeat > sc.startBeat {
			sc.sound.ScheduleStop(e.transport.beatsToFrames(sc.endBeat))
		}
		if err := sc.sound.Start(); err != nil {
			return err
		}
	}

	w, err := wav.Create(outPath, uint32(e.transport.sampleRate), uint16(ch))
	if err != nil {
		return err
	}
	defer w.Close()

	// Pull audio in chunks and write it out until we've produced totalFrames.
	// Before each chunk we sample the automation lanes at the chunk's start
	// position, so renders include fades/sweeps and stay fully deterministic
	// (no wall-clock ticker like live playback uses).
	p := e.store.Get()
	fpb := e.transport.framesPerBeat()
	const chunkFrames = 4096
	buf := make([]float32, chunkFrames*int(ch))
	var done uint64
	for done < totalFrames {
		if len(e.lanes) > 0 && fpb > 0 {
			beat := float64(done) / fpb
			for _, ln := range e.lanes {
				e.applyLaneLocked(p, ln, beat)
			}
		}

		want := uint64(chunkFrames)
		if rem := totalFrames - done; rem < want {
			want = rem
		}
		n, err := e.audio.ReadFrames(buf[:want*uint64(ch)], ch)
		if err != nil {
			return err
		}
		if n == 0 {
			break // engine produced nothing more
		}
		if err := w.WriteFloat32(buf[:n*uint64(ch)]); err != nil {
			return err
		}
		done += n
	}

	dur := w.Duration()
	if err := w.Close(); err != nil {
		return err
	}
	log.Printf("[render] wrote %s — %.2fs, %d Hz", outPath, dur, e.transport.sampleRate)
	return nil
}
