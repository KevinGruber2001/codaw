// Package state holds the runtime state of a loaded CodaW project.
// It is the single source of truth at runtime — the audio engine, file watcher,
// and UI all read from and write to this package, never directly to each other.
package state

// ─────────────────────────────────────────────
//  EventType
// ─────────────────────────────────────────────

// EventType is a string enum identifying what kind of change happened.
//
// Why a string and not an int (iota)?
// String enums are self-documenting in logs and over the wire (future IPC).
// "track.gain_changed" is immediately readable in a log file.
// An iota constant like `42` tells you nothing without a lookup table.
//
// The dot-namespace convention (domain.action) scales well —
// you can filter by prefix: "track." catches all track events.
type EventType string

const (
	// EventTrackGainChanged fires when a track's gain value changes.
	EventTrackGainChanged EventType = "track.gain_changed"

	// EventTrackPanChanged fires when a track's pan value changes.
	EventTrackPanChanged EventType = "track.pan_changed"

	// EventTrackMuteChanged fires when a track is muted or unmuted.
	EventTrackMuteChanged EventType = "track.mute_changed"

	// EventTrackSoloChanged fires when a track's solo state changes.
	EventTrackSoloChanged EventType = "track.solo_changed"

	// EventClipGainChanged fires when a single clip's gain trim changes.
	// Hot-swappable: the engine adjusts that clip's volume live, no rebuild.
	EventClipGainChanged EventType = "clip.gain_changed"

	// EventFXParamChanged fires when any FX parameter on any track/bus/master changes.
	EventFXParamChanged EventType = "fx.param_changed"

	// EventBusGainChanged fires when a bus gain changes.
	EventBusGainChanged EventType = "bus.gain_changed"

	// EventMasterGainChanged fires when the master gain changes.
	EventMasterGainChanged EventType = "master.gain_changed"

	// EventTransportBPMChanged fires when the BPM changes.
	// This is significant for the audio engine — it needs to recalculate
	// all beat-to-sample offsets for every clip.
	EventTransportBPMChanged EventType = "transport.bpm_changed"

	// EventStructureChanged fires when the project structure changes —
	// a track was added/removed, a clip was added/removed, an FX unit
	// was added/removed. The audio engine needs to rebuild its graph.
	EventStructureChanged EventType = "project.structure_changed"

	// EventProjectReloaded fires when the entire project is reloaded from disk.
	// This is a "nuclear" event — subscribers should treat it as a full reset.
	EventProjectReloaded EventType = "project.reloaded"
)

// ─────────────────────────────────────────────
//  Event
// ─────────────────────────────────────────────

// Event is what gets sent to subscribers when state changes.
//
// Design decision: why not just send the full Project on every change?
// Two reasons:
//  1. Performance — the audio engine runs in a tight loop. Sending and
//     copying a full Project struct on every gain tweak is wasteful.
//  2. Precision — subscribers can filter by Type and only react to what
//     they care about. The audio engine ignores EventTransportBPMChanged
//     differently than a metronome UI widget would.
//
// The Payload field uses `any` (Go 1.18+ alias for interface{}).
// Each EventType has a corresponding payload type defined below —
// subscribers type-assert to get the concrete payload:
//
//	case EventTrackGainChanged:
//	    p := event.Payload.(TrackGainPayload)
//	    engine.SetTrackGain(p.TrackID, p.Gain)
type Event struct {
	// Type identifies what changed.
	Type EventType

	// Payload carries the specific changed values.
	// Type-assert to the corresponding payload struct.
	Payload any
}

// ─────────────────────────────────────────────
//  Payload types
// ─────────────────────────────────────────────
// One payload struct per event type that carries data.
// Keeping them small and specific means subscribers only get
// what they need — no digging through a large struct.

// TrackGainPayload is the payload for EventTrackGainChanged.
type TrackGainPayload struct {
	TrackID string
	Gain    float64 // new gain value in dB
}

// TrackPanPayload is the payload for EventTrackPanChanged.
type TrackPanPayload struct {
	TrackID string
	Pan     float64 // new pan value, -1.0 to 1.0
}

// TrackMutePayload is the payload for EventTrackMuteChanged.
type TrackMutePayload struct {
	TrackID string
	Muted   bool
}

// TrackSoloPayload is the payload for EventTrackSoloChanged.
type TrackSoloPayload struct {
	TrackID string
	Soloed  bool
}

// ClipGainPayload is the payload for EventClipGainChanged.
type ClipGainPayload struct {
	TrackID   string
	ClipIndex int     // position in the track's clip list (0-based)
	Gain      float64 // new clip gain trim in dB
}

// FXParamPayload is the payload for EventFXParamChanged.
// It carries enough information to find the exact parameter that changed:
// which object (track/bus/master), which fx slot, which param key.
type FXParamPayload struct {
	// OwnerID is the ID of the track or bus, or "master" for the master chain.
	OwnerID string

	// FXIndex is the position of the effect in the chain (0-based).
	FXIndex int

	// Key is the parameter name (e.g. "wet", "room_size", "threshold").
	Key string

	// Value is the new parameter value.
	Value float64
}

// BusGainPayload is the payload for EventBusGainChanged.
type BusGainPayload struct {
	BusID string
	Gain  float64
}

// MasterGainPayload is the payload for EventMasterGainChanged.
type MasterGainPayload struct {
	Gain float64
}

// BPMPayload is the payload for EventTransportBPMChanged.
type BPMPayload struct {
	BPM float64
}
