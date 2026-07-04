package state

import "github.com/KevinGruber2001/codaw/internal/project"

// ─────────────────────────────────────────────
//  What is a Mutation?
// ─────────────────────────────────────────────
//
// A Mutation is a function that takes a *project.Project and modifies it.
// That's it. The simplicity is intentional.
//
// Why this pattern instead of direct field access?
//
// Compare these two approaches:
//
//   // Direct access — works but has problems:
//   store.project.Tracks[2].Gain = -3.0
//   // Problems:
//   // 1. Caller needs to know the array index (fragile, order-dependent)
//   // 2. No locking — data race if audio engine reads simultaneously
//   // 3. No event emitted — subscribers never know it changed
//   // 4. No write-back to TOML file
//
//   // Mutation pattern — all problems solved:
//   store.Apply(SetTrackGain("kick", -3.0))
//   // The store handles: locking, applying, emitting event, writing TOML
//
// Mutations are also pure functions — they don't call Apply themselves.
// This makes them easy to test: just create a Project, run a mutation,
// check the result. No store, no goroutines, no side effects.
//
// This pattern is inspired by Redux (JavaScript state management) and
// the command pattern from classic OOP design patterns.

// Mutation is a function that modifies a project in place.
// It receives a pointer to the Project so it can modify fields directly.
// It returns an Event describing what changed — the store will emit this
// to all subscribers after applying the mutation.
//
// Returning the Event from the mutation (rather than having the store
// figure it out) keeps event construction close to the mutation logic
// where the context is clear.
type Mutation func(*project.Project) Event

// ─────────────────────────────────────────────
//  Track mutations
// ─────────────────────────────────────────────

// SetTrackGain returns a Mutation that sets a track's gain.
//
// Note the pattern: a constructor function that returns a Mutation.
// This is a closure — the returned function "closes over" trackID and gain,
// capturing them in its scope. When the store calls the mutation later,
// those values are still available even though SetTrackGain has returned.
//
// This is idiomatic Go for parameterized behavior — equivalent to
// a class with a single method in OOP, but much lighter.
func SetTrackGain(trackID string, gain float64) Mutation {
	return func(p *project.Project) Event {
		for _, t := range p.Tracks {
			if t.ID == trackID {
				t.Gain = gain
				// Return the event immediately — we found the track.
				return Event{
					Type:    EventTrackGainChanged,
					Payload: TrackGainPayload{TrackID: trackID, Gain: gain},
				}
			}
		}
		// Track not found — return a zero Event.
		// The store checks for zero events and skips emitting.
		// This is safer than panicking — unknown IDs are silently ignored.
		return Event{}
	}
}

// SetTrackPan returns a Mutation that sets a track's stereo pan position.
func SetTrackPan(trackID string, pan float64) Mutation {
	return func(p *project.Project) Event {
		for _, t := range p.Tracks {
			if t.ID == trackID {
				t.Pan = pan
				return Event{
					Type:    EventTrackPanChanged,
					Payload: TrackPanPayload{TrackID: trackID, Pan: pan},
				}
			}
		}
		return Event{}
	}
}

// SetTrackMute returns a Mutation that mutes or unmutes a track.
func SetTrackMute(trackID string, muted bool) Mutation {
	return func(p *project.Project) Event {
		for _, t := range p.Tracks {
			if t.ID == trackID {
				t.Mute = muted
				return Event{
					Type:    EventTrackMuteChanged,
					Payload: TrackMutePayload{TrackID: trackID, Muted: muted},
				}
			}
		}
		return Event{}
	}
}

// SetTrackSolo returns a Mutation that solos or un-solos a track.
func SetTrackSolo(trackID string, soloed bool) Mutation {
	return func(p *project.Project) Event {
		for _, t := range p.Tracks {
			if t.ID == trackID {
				t.Solo = soloed
				return Event{
					Type:    EventTrackSoloChanged,
					Payload: TrackSoloPayload{TrackID: trackID, Soloed: soloed},
				}
			}
		}
		return Event{}
	}
}

// ─────────────────────────────────────────────
//  FX mutations
// ─────────────────────────────────────────────

// SetTrackFXParam returns a Mutation that sets a single parameter
// on an effect in a track's FX chain.
//
// ownerID can be a track ID, a bus ID, or "master".
// fxIndex is the 0-based position in the FX chain.
// key is the parameter name (e.g. "wet", "room_size").
// value is the new parameter value.
func SetFXParam(ownerID string, fxIndex int, key string, value float64) Mutation {
	return func(p *project.Project) Event {
		// Try tracks first
		if chain := fxChainFor(p, ownerID); chain != nil {
			if fxIndex >= 0 && fxIndex < len(*chain) {
				if (*chain)[fxIndex].Params == nil {
					(*chain)[fxIndex].Params = make(map[string]any)
				}
				(*chain)[fxIndex].Params[key] = value
				return Event{
					Type: EventFXParamChanged,
					Payload: FXParamPayload{
						OwnerID: ownerID,
						FXIndex: fxIndex,
						Key:     key,
						Value:   value,
					},
				}
			}
		}
		return Event{}
	}
}

// fxChainFor is a helper that returns a pointer to the FX slice
// for a given owner ID (track, bus, or "master").
//
// Returning a pointer to the slice (not the slice itself) is important —
// if we returned the slice by value, modifications wouldn't affect the
// original Project. A pointer lets us modify in place.
//
// Why a helper? Because SetFXParam needs to find the FX chain regardless
// of whether the owner is a track, bus, or master. Centralizing that
// logic here means we only write it once.
func fxChainFor(p *project.Project, ownerID string) *[]project.FX {
	if ownerID == "master" {
		if p.Master != nil {
			return &p.Master.FX
		}
		return nil
	}

	for _, t := range p.Tracks {
		if t.ID == ownerID {
			return &t.FX
		}
	}

	for _, b := range p.Buses {
		if b.ID == ownerID {
			return &b.FX
		}
	}

	return nil
}

// ─────────────────────────────────────────────
//  Bus mutations
// ─────────────────────────────────────────────

// SetBusGain returns a Mutation that sets a bus's gain.
func SetBusGain(busID string, gain float64) Mutation {
	return func(p *project.Project) Event {
		for _, b := range p.Buses {
			if b.ID == busID {
				b.Gain = gain
				return Event{
					Type:    EventBusGainChanged,
					Payload: BusGainPayload{BusID: busID, Gain: gain},
				}
			}
		}
		return Event{}
	}
}

// ─────────────────────────────────────────────
//  Master mutations
// ─────────────────────────────────────────────

// SetMasterGain returns a Mutation that sets the master output gain.
func SetMasterGain(gain float64) Mutation {
	return func(p *project.Project) Event {
		if p.Master != nil {
			p.Master.Gain = gain
			return Event{
				Type:    EventMasterGainChanged,
				Payload: MasterGainPayload{Gain: gain},
			}
		}
		return Event{}
	}
}

// ─────────────────────────────────────────────
//  Transport mutations
// ─────────────────────────────────────────────

// SetBPM returns a Mutation that changes the project BPM.
// This is significant — the audio engine must recalculate all
// beat-to-sample offsets when BPM changes.
func SetBPM(bpm float64) Mutation {
	return func(p *project.Project) Event {
		p.Transport.BPM = bpm
		return Event{
			Type:    EventTransportBPMChanged,
			Payload: BPMPayload{BPM: bpm},
		}
	}
}

// ─────────────────────────────────────────────
//  Structural mutations (require engine graph rebuild)
// ─────────────────────────────────────────────

// ReplaceTrack returns a Mutation that replaces an entire track's data.
// Used by the file watcher when a track file is reloaded from disk and
// the structural content (clips, fx chain order) has changed.
//
// This is a "nuclear" track mutation — it replaces everything.
// For single-field changes (gain, pan) use the specific mutations above,
// which produce targeted events that the engine can apply without rebuilding.
func ReplaceTrack(updated *project.Track) Mutation {
	return func(p *project.Project) Event {
		for i, t := range p.Tracks {
			if t.ID == updated.ID {
				// Preserve the SourceFile — it's set by the loader,
				// not present in the TOML, so the new parsed track won't have it.
				updated.SourceFile = t.SourceFile
				p.Tracks[i] = updated
				return Event{
					Type:    EventStructureChanged,
					Payload: nil, // subscriber should re-read the full project
				}
			}
		}
		return Event{}
	}
}

// ReplaceBus replaces an entire bus's data. Same rationale as ReplaceTrack.
func ReplaceBus(updated *project.Bus) Mutation {
	return func(p *project.Project) Event {
		for i, b := range p.Buses {
			if b.ID == updated.ID {
				updated.SourceFile = b.SourceFile
				p.Buses[i] = updated
				return Event{Type: EventStructureChanged}
			}
		}
		return Event{}
	}
}

// ReplaceMaster replaces the master settings entirely.
func ReplaceMaster(updated *project.Master) Mutation {
	return func(p *project.Project) Event {
		updated.SourceFile = p.Master.SourceFile
		p.Master = updated
		return Event{Type: EventStructureChanged}
	}
}
