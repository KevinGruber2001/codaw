package engine

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/KevinGruber2001/codaw/internal/project"
)

// Automation drives a parameter over time. The project stores curves (beat →
// value); the engine samples them against the playhead and pushes values into
// the same group/effect setters that hot-reload uses. So a gain fade is really
// just "call SetGainDB many times per second with interpolated values".

// autoKind is which kind of parameter a lane drives.
type autoKind int

const (
	autoGain autoKind = iota // track gain (dB)
	autoPan                  // track pan (-1..1)
	autoFX                   // an FX parameter
)

// autoLane is a resolved automation curve ready to apply: which track, which
// parameter, and the sorted points to interpolate.
type autoLane struct {
	trackID string
	kind    autoKind
	fxIndex int    // for autoFX
	fxKey   string // for autoFX
	points  []project.AutomationPoint
}

// parseTarget turns a target string ("gain", "pan", "fx[0].wet") into a lane
// kind plus, for FX, the chain index and param key.
func parseTarget(target string) (autoKind, int, string, error) {
	switch target {
	case "gain":
		return autoGain, 0, "", nil
	case "pan":
		return autoPan, 0, "", nil
	}

	// fx[<index>].<key>
	if strings.HasPrefix(target, "fx[") {
		rest := target[len("fx["):]
		end := strings.IndexByte(rest, ']')
		if end < 0 || end+1 >= len(rest) || rest[end+1] != '.' {
			return 0, 0, "", fmt.Errorf("malformed fx target %q (want fx[N].key)", target)
		}
		idx, err := strconv.Atoi(rest[:end])
		if err != nil {
			return 0, 0, "", fmt.Errorf("bad fx index in %q: %w", target, err)
		}
		key := rest[end+2:]
		if key == "" {
			return 0, 0, "", fmt.Errorf("missing fx param key in %q", target)
		}
		return autoFX, idx, key, nil
	}

	return 0, 0, "", fmt.Errorf("unknown automation target %q", target)
}

// buildLanes resolves every track's automation into engine lanes. Bad targets
// are logged and skipped rather than failing the whole graph. Caller holds e.mu.
func (e *Engine) buildLanes(p *project.Project) {
	for _, t := range p.Tracks {
		for _, a := range t.Automation {
			kind, idx, key, err := parseTarget(a.Target)
			if err != nil {
				log.Printf("[engine] track %q: skipping automation — %v", t.ID, err)
				continue
			}
			// Copy + sort the points by beat so interpolation can scan linearly.
			pts := make([]project.AutomationPoint, len(a.Points))
			copy(pts, a.Points)
			sort.Slice(pts, func(i, j int) bool { return pts[i].Beat < pts[j].Beat })

			e.lanes = append(e.lanes, autoLane{
				trackID: t.ID,
				kind:    kind,
				fxIndex: idx,
				fxKey:   key,
				points:  pts,
			})
		}
	}
	if len(e.lanes) > 0 {
		log.Printf("[engine] %d automation lane(s) active", len(e.lanes))
	}
}

// interpolate returns the curve value at beat. Before the first point it holds
// the first value; after the last it holds the last value; in between it's a
// straight line (linear ramp) between the surrounding points.
func interpolate(pts []project.AutomationPoint, beat float64) float64 {
	if len(pts) == 0 {
		return 0
	}
	if beat <= pts[0].Beat {
		return pts[0].Value
	}
	last := pts[len(pts)-1]
	if beat >= last.Beat {
		return last.Value
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		if beat >= a.Beat && beat <= b.Beat {
			span := b.Beat - a.Beat
			if span <= 0 {
				return b.Value
			}
			f := (beat - a.Beat) / span
			return a.Value + f*(b.Value-a.Value)
		}
	}
	return last.Value
}

// positionBeatsLocked returns the current playhead position in beats, derived
// from the engine's global sample clock relative to where playback started.
// Caller holds e.mu.
func (e *Engine) positionBeatsLocked() float64 {
	now := e.audio.TimeFrames()
	if now <= e.playBase {
		return 0
	}
	fpb := e.transport.framesPerBeat()
	if fpb <= 0 {
		return 0
	}
	return float64(now-e.playBase) / fpb
}

// applyLaneLocked samples one lane at the given beat and pushes the value into
// the live graph. Caller holds e.mu.
func (e *Engine) applyLaneLocked(p *project.Project, ln autoLane, beat float64) {
	val := interpolate(ln.points, beat)

	switch ln.kind {
	case autoGain:
		g := e.tracks[ln.trackID]
		if g == nil {
			return
		}
		// Respect mute/solo: automation shouldn't un-silence a muted track.
		if t := findTrack(p, ln.trackID); t != nil && trackAudible(p, t) {
			g.SetGainDB(val)
		} else {
			g.SetVolume(0)
		}

	case autoPan:
		if g := e.tracks[ln.trackID]; g != nil {
			g.SetPan(val)
		}

	case autoFX:
		chain := e.fx[ln.trackID]
		if ln.fxIndex >= 0 && ln.fxIndex < len(chain) && chain[ln.fxIndex] != nil {
			chain[ln.fxIndex].SetParam(ln.fxKey, val)
		}
	}
}

// automationLoop is the live-playback driver: a wall-clock ticker that samples
// every lane against the playhead ~50×/sec. (Offline rendering doesn't use this
// — Render samples lanes per audio chunk instead, so it's deterministic.)
func (e *Engine) automationLoop() {
	defer close(e.autoDone)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-e.autoStop:
			return
		case <-ticker.C:
			e.tickAutomation()
		}
	}
}

// tickAutomation applies all lanes at the current playhead. No-op when stopped
// or when there's nothing to automate.
func (e *Engine) tickAutomation() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.playing || len(e.lanes) == 0 {
		return
	}
	p := e.store.Get()
	beat := e.positionBeatsLocked()
	for _, ln := range e.lanes {
		e.applyLaneLocked(p, ln, beat)
	}
}
