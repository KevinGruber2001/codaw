package engine

import "github.com/KevinGruber2001/codaw/internal/project"

// This file holds the mixing rules that can't be expressed as a single group
// setter — specifically mute and solo, which are *relational*: turning solo on
// for one track changes whether every *other* track is heard.

// anySoloed reports whether at least one track has Solo enabled.
func anySoloed(p *project.Project) bool {
	for _, t := range p.Tracks {
		if t.Solo {
			return true
		}
	}
	return false
}

// trackAudible applies the mute/solo rules for a single track:
//
//   - A muted track is never heard.
//   - If *any* track is soloed, only soloed tracks are heard (everything else
//     is implicitly silenced). This is how solo works on a real mixer.
//   - Otherwise the track is heard.
func trackAudible(p *project.Project, t *project.Track) bool {
	if t.Mute {
		return false
	}
	if anySoloed(p) && !t.Solo {
		return false
	}
	return true
}

// applyTrackMix sets a track group's gain and pan from the project values,
// then folds in the mute/solo decision. When the track should be silent we
// drop the group to linear volume 0 instead of using its dB gain.
func (e *Engine) applyTrackMix(p *project.Project, t *project.Track) {
	g := e.tracks[t.ID]
	if g == nil {
		return
	}
	g.SetPan(t.Pan)
	if trackAudible(p, t) {
		g.SetGainDB(t.Gain)
	} else {
		g.SetVolume(0)
	}
}

// applyAllTrackMix re-evaluates every track's mix. We need the whole pass
// (not just the changed track) whenever solo changes, because one track's solo
// state determines whether all the others are audible.
func (e *Engine) applyAllTrackMix(p *project.Project) {
	for _, t := range p.Tracks {
		e.applyTrackMix(p, t)
	}
}
