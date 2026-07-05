// Package engine turns a loaded project into sound. It owns a miniaudio engine
// (via internal/audio), builds a routing graph of groups and clips from the
// project, plays it back on the timeline, and — the headline feature —
// subscribes to the state Store so edits to the TOML files change the audio
// live, with no restart.
//
// Layering:
//
//	engine  →  state (Store, events)  +  audio (miniaudio wrappers)  →  project
//
// engine never touches C directly; all that lives in internal/audio.
package engine

import (
	"log"
	"math"
	"sync"

	"github.com/KevinGruber2001/codaw/internal/audio"
	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/state"
)

// Engine plays a project and keeps it in sync with the Store.
type Engine struct {
	audio *audio.Engine
	store *state.Store

	// mu guards the mutable graph state below, because store events arrive on a
	// background goroutine while Play/Stop/Close may be called from main.
	mu        sync.Mutex
	transport transport
	master    *audio.Group
	buses     map[string]*audio.Group // bus ID  → group
	tracks    map[string]*audio.Group // track ID → group
	clips     []scheduledClip
	playing   bool

	// fx maps an owner ("master", a bus ID, or a track ID) to its effect
	// chain. The slice is index-aligned with that owner's project FX array —
	// entries are nil for effect types we don't implement yet (e.g. compressor),
	// so FXIndex from an event always lines up.
	fx map[string][]*audio.Effect

	// automation
	lanes []autoLane // resolved automation curves

	// transport / playhead
	//
	// The engine's global clock is a monotonic frame counter. To know "which
	// beat is playing", we remember which clock frame corresponds to which
	// timeline beat at the moment playback (re)started:
	//
	//   position = playStartBeat + (clock - playFrame) / framesPerBeat
	//
	// Storing (frame, beat) as a pair — instead of "frame where beat 0 sits" —
	// avoids unsigned underflow when seeking far into the timeline early in
	// the engine's life (beat 0 could map to a negative clock frame).
	playFrame     uint64  // clock frame where playStartBeat sits
	playStartBeat float64 // timeline beat at playFrame
	pendingBeat   float64 // where the NEXT Play starts (set by Seek/Stop)

	// live-update plumbing
	events      chan state.Event
	unsubscribe func()
	listenDone  chan struct{} // closed when the listen goroutine exits

	// automation loop plumbing (live playback only; nil for offline render)
	autoStop chan struct{} // close to stop the automation ticker
	autoDone chan struct{} // closed when the automation goroutine exits
}

// New boots the audio device and starts listening for state changes. Call Load
// next to build the graph, then Play.
func New(store *state.Store) (*Engine, error) {
	a, err := audio.NewEngine()
	if err != nil {
		return nil, err
	}
	e := &Engine{
		audio:      a,
		store:      store,
		buses:      make(map[string]*audio.Group),
		tracks:     make(map[string]*audio.Group),
		fx:         make(map[string][]*audio.Effect),
		events:     make(chan state.Event, 64), // buffered so emit never drops on us
		listenDone: make(chan struct{}),
		autoStop:   make(chan struct{}),
		autoDone:   make(chan struct{}),
	}
	e.unsubscribe = store.Subscribe(e.events)
	go e.listen()
	go e.automationLoop()
	return e, nil
}

// Load builds the audio graph from the current project state. Safe to call
// again later (e.g. after a structural change) — it tears down first.
func (e *Engine) Load() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reload()
}

// reload rebuilds the graph from scratch, preserving the playhead: if we were
// playing at beat N when a structural/BPM change landed, playback resumes at
// beat N on the new graph instead of snapping back to 0. Caller must hold e.mu.
func (e *Engine) reload() error {
	wasPlaying := e.playing
	resumeAt := e.pendingBeat
	if wasPlaying {
		resumeAt = e.positionBeatsLocked()
	}
	e.stopLocked()
	e.teardownGraph()

	p := e.store.Get()
	// Beat→frame math uses the *device* rate (what the global clock ticks at),
	// not the project's requested rate. See transport.go.
	e.transport = transport{
		sampleRate: int(e.audio.SampleRate()),
		bpm:        p.Transport.BPM,
	}
	if err := e.buildGraph(p); err != nil {
		return err
	}
	if wasPlaying {
		return e.playFromLocked(resumeAt)
	}
	e.pendingBeat = resumeAt
	return nil
}

// Play starts (or resumes) playback from the pending position — beat 0 on a
// fresh engine, or wherever Stop/Seek last left the playhead.
func (e *Engine) Play() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.playFromLocked(e.pendingBeat)
}

// Seek moves the playhead to the given beat. If playing, playback jumps there
// live; if stopped, the position is remembered for the next Play.
func (e *Engine) Seek(beat float64) error {
	if beat < 0 {
		beat = 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.playing {
		return e.playFromLocked(beat)
	}
	e.pendingBeat = beat
	return nil
}

// PositionBeats reports the current playhead position in beats — live during
// playback, or the pending start position while stopped. Safe for any
// goroutine (the serve loop polls this for position events).
func (e *Engine) PositionBeats() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.playing {
		return e.positionBeatsLocked()
	}
	return e.pendingBeat
}

// Playing reports whether the transport is currently running.
func (e *Engine) Playing() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.playing
}

// playFromLocked (re)starts playback with the playhead at fromBeat.
// Caller must hold e.mu.
//
// Every clip falls into one of three cases relative to the playhead:
//
//	ends before it   → stays silent (nothing to do)
//	starts after it  → scheduled at its normal offset, shifted by -fromBeat
//	spans it         → the interesting one: the sound must start immediately
//	                   but from the correct position INSIDE its source file,
//	                   as if it had been playing all along
func (e *Engine) playFromLocked(fromBeat float64) error {
	// Unarm everything first — leftover schedules from a previous Play would
	// otherwise fire alongside the new ones.
	e.stopLocked()

	// Short lead-in (~100 ms) so every clip is armed before the clock reaches
	// the first start frame. base is the clock frame where fromBeat sits.
	lead := uint64(e.transport.sampleRate / 10)
	base := e.audio.TimeFrames() + lead
	e.playFrame = base
	e.playStartBeat = fromBeat
	e.pendingBeat = fromBeat

	secPerBeat := 60.0 / e.transport.bpm

	for _, sc := range e.clips {
		if sc.endBeat <= fromBeat {
			continue // already over at this playhead
		}
		stopFrame := base + e.transport.beatsToFrames(sc.endBeat-fromBeat)

		if sc.startBeat >= fromBeat {
			// Starts in the future: rewind the sound to its in-point (its
			// cursor may sit anywhere after an earlier Play) and schedule.
			if err := sc.sound.SeekToSecond(sc.offsetSec); err != nil {
				return err
			}
			sc.sound.ScheduleStart(base + e.transport.beatsToFrames(sc.startBeat-fromBeat))
		} else {
			// Playhead lands inside this clip: figure out where in the FILE
			// we'd be after (fromBeat - startBeat) beats of playback.
			lenSec, err := sc.sound.LengthSeconds()
			if err != nil {
				return err
			}
			filePos, ok := clipFilePosition(sc.offsetSec, lenSec, sc.loop, (fromBeat-sc.startBeat)*secPerBeat)
			if !ok {
				continue // non-looping clip whose audio already ran out
			}
			if err := sc.sound.SeekToSecond(filePos); err != nil {
				return err
			}
			sc.sound.ScheduleStart(base)
		}

		sc.sound.ScheduleStop(stopFrame)
		if err := sc.sound.Start(); err != nil {
			return err
		}
	}
	e.playing = true
	return nil
}

// clipFilePosition maps "elapsed seconds since the clip's timeline start" to a
// position inside the source file, honouring the in-point offset and looping.
//
// Playback walks the file as: offset → end, then (if looping) 0 → end, 0 → end…
// (miniaudio loops to the file start, not the offset — see Sound.SeekToSecond).
// The math simply retraces that walk. Returns ok=false when a non-looping
// clip's audio is already exhausted at this position. Pure function so the
// wrap-around logic is unit-testable without an audio device.
func clipFilePosition(offsetSec, lenSec float64, loop bool, elapsedSec float64) (float64, bool) {
	if lenSec <= 0 {
		return 0, false
	}

	firstPass := lenSec - offsetSec // seconds of audio in the first pass
	if elapsedSec < firstPass {
		return offsetSec + elapsedSec, true
	}
	if !loop {
		return 0, false // ran out of audio; silence fills the clip window
	}
	return math.Mod(elapsedSec-firstPass, lenSec), true
}

// Stop halts all clips and parks the playhead at the stop position, so a
// following Play resumes where you left off (pause semantics). Use Seek(0)
// for return-to-start.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.playing {
		e.pendingBeat = e.positionBeatsLocked()
	}
	e.stopLocked()
}

func (e *Engine) stopLocked() {
	for _, sc := range e.clips {
		_ = sc.sound.Stop()
	}
	e.playing = false
}

// Close stops playback, tears down the graph, detaches from the Store, and
// shuts down the audio device. The Engine must not be used afterward.
//
// Ordering matters: the background goroutines (listen, automation) call into the
// audio engine, so they must be fully stopped before we free it — otherwise a
// late tick would touch freed C memory and crash. We wait on their done
// channels *without* holding e.mu, since they take e.mu themselves.
func (e *Engine) Close() {
	if e.unsubscribe != nil {
		e.unsubscribe() // remove our channel from the Store first...
		close(e.events) // ...now no emit can send to it; listen drains + exits.
		<-e.listenDone  // wait for the listen goroutine to finish.
		e.unsubscribe = nil
	}
	if e.autoStop != nil {
		close(e.autoStop) // stop the automation ticker...
		<-e.autoDone      // ...and wait for it to return.
		e.autoStop = nil
	}

	e.mu.Lock()
	e.stopLocked()
	e.teardownGraph()
	e.mu.Unlock()

	e.audio.Close()
}

// ─────────────────────────────────────────────
//  Live updates
// ─────────────────────────────────────────────

// listen consumes Store events on a background goroutine and applies each one
// to the live audio graph. This is what makes editing a TOML file audible
// instantly: watcher → Store.Apply → event → here → group setter.
func (e *Engine) listen() {
	defer close(e.listenDone)
	for ev := range e.events {
		e.handle(ev)
	}
}

func (e *Engine) handle(ev state.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// The mutation has already been applied to the project, so Get() reflects
	// the new values. We read from it rather than trusting only the payload,
	// which keeps mute/solo (relational) decisions correct.
	p := e.store.Get()

	switch ev.Type {
	case state.EventTrackGainChanged:
		if t := findTrack(p, ev.Payload.(state.TrackGainPayload).TrackID); t != nil {
			e.applyTrackMix(p, t) // gain respects mute/solo
		}

	case state.EventTrackPanChanged:
		pl := ev.Payload.(state.TrackPanPayload)
		if g := e.tracks[pl.TrackID]; g != nil {
			g.SetPan(pl.Pan)
		}

	case state.EventTrackMuteChanged:
		if t := findTrack(p, ev.Payload.(state.TrackMutePayload).TrackID); t != nil {
			e.applyTrackMix(p, t)
		}

	case state.EventTrackSoloChanged:
		// Solo is relational — re-evaluate every track.
		e.applyAllTrackMix(p)

	case state.EventBusGainChanged:
		pl := ev.Payload.(state.BusGainPayload)
		if g := e.buses[pl.BusID]; g != nil {
			g.SetGainDB(pl.Gain)
		}

	case state.EventMasterGainChanged:
		if e.master != nil {
			e.master.SetGainDB(ev.Payload.(state.MasterGainPayload).Gain)
		}

	case state.EventTransportBPMChanged:
		// Tempo affects every clip's scheduled position. Rebuilding re-runs the
		// beat→frame math and reschedules.
		e.transport.bpm = ev.Payload.(state.BPMPayload).BPM
		log.Printf("[engine] bpm changed → rebuilding timeline")
		if err := e.reload(); err != nil {
			log.Printf("[engine] rebuild after bpm change failed: %v", err)
		}

	case state.EventClipGainChanged:
		pl := ev.Payload.(state.ClipGainPayload)
		for _, sc := range e.clips {
			if sc.trackID == pl.TrackID && sc.clipIndex == pl.ClipIndex {
				sc.sound.SetGainDB(pl.Gain)
				log.Printf("[engine] clip %s[%d] gain = %.1f dB", pl.TrackID, pl.ClipIndex, pl.Gain)
				break
			}
		}

	case state.EventFXParamChanged:
		pl := ev.Payload.(state.FXParamPayload)
		if chain := e.fx[pl.OwnerID]; pl.FXIndex >= 0 && pl.FXIndex < len(chain) {
			if ef := chain[pl.FXIndex]; ef != nil {
				ef.SetParam(pl.Key, pl.Value)
				log.Printf("[engine] fx %s[%d] %s.%s = %.3f", pl.OwnerID, pl.FXIndex, ef.Kind(), pl.Key, pl.Value)
			}
		}

	case state.EventStructureChanged, state.EventProjectReloaded:
		log.Printf("[engine] structure changed → rebuilding graph")
		if err := e.reload(); err != nil {
			log.Printf("[engine] rebuild failed: %v", err)
		}

	default:
		log.Printf("[engine] unhandled event %s", ev.Type)
	}
}

// findTrack returns the track with the given ID, or nil.
func findTrack(p *project.Project, id string) *project.Track {
	for _, t := range p.Tracks {
		if t.ID == id {
			return t
		}
	}
	return nil
}
