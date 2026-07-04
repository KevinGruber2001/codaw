// Package project handles loading, parsing, and validating CodaW project files.
// A project is split across multiple TOML files — one root project.toml that
// references separate track, bus, and master files.
package project

import "fmt"

// ─────────────────────────────────────────────
//  FX
// ─────────────────────────────────────────────

// FX represents a single effects unit in a chain.
// The Type field maps to a registered plugin (e.g. "reverb", "eq_3band").
// Params holds every other key from the TOML table — we use a map here
// instead of a fixed struct because each plugin type has different parameters.
// The engine reads plugin-specific params from this map when it builds the
// effect (e.g. "low_db", "room_size").
//
// Example TOML:
//
//	[[fx]]
//	type      = "reverb"
//	room_size = 0.65
//	wet       = 0.4
type FX struct {
	// Type is the only required field — it identifies which plugin to load.
	Type string

	// Params captures all other key/value pairs in the [[fx]] table, keyed by
	// TOML key. Values arrive as the types BurntSushi decodes them to
	// (int64 for whole numbers, float64 for decimals, string, bool, ...).
	Params map[string]any
}

// UnmarshalTOML lets BurntSushi hand us the raw [[fx]] table so we can split it
// into Type + Params ourselves.
//
// Why this is necessary: BurntSushi/toml has no "capture the leftover keys"
// struct tag (the `,remain` tag belongs to a different library and was silently
// ignored, dropping every FX parameter). Implementing toml.Unmarshaler is the
// supported way to take over decoding for a type — BurntSushi passes us the
// decoded table as a map[string]interface{}, and we route the keys.
func (f *FX) UnmarshalTOML(data any) error {
	table, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("fx: expected a table, got %T", data)
	}

	f.Params = make(map[string]any, len(table))
	for key, val := range table {
		if key == "type" {
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("fx: 'type' must be a string, got %T", val)
			}
			f.Type = s
			continue
		}
		f.Params[key] = val
	}
	return nil
}

// ─────────────────────────────────────────────
//  Clip
// ─────────────────────────────────────────────

// Clip represents a single piece of audio placed on the timeline.
// Multiple clips can live on one track, placed at different beat positions.
//
// Example TOML:
//
//	[[clip]]
//	file  = "samples/kick_01.wav"
//	start = 0.0
//	end   = 16.0
//	loop  = true
//	gain  = 0.0
type Clip struct {
	// File is the path to the audio file, relative to the project root.
	// Supported formats depend on miniaudio: WAV, MP3, FLAC, OGG.
	File string `toml:"file" validate:"required"`

	// Start is the beat position where this clip begins playing.
	// Beat 0.0 = the very start of the project.
	// Beat 4.0 = the start of bar 2 in 4/4 time.
	Start float64 `toml:"start" validate:"min=0"`

	// End is the beat position where this clip stops.
	// If the audio file is shorter than (end - start) beats and Loop is true,
	// the file loops to fill the range. If Loop is false, silence fills the rest.
	End float64 `toml:"end" validate:"gtfield=Start"`

	// Offset is the in-point: how many seconds into the source file playback
	// begins. 0 = the start of the file. This is what makes non-destructive
	// trimming and splitting possible — two clips can reference the same file at
	// different offsets without ever touching the audio data.
	Offset float64 `toml:"offset" validate:"min=0"`

	// Loop controls whether the clip loops to fill the [Start, End] range.
	Loop bool `toml:"loop"`

	// Gain is a per-clip volume trim in dB, applied before the track gain.
	// Useful for balancing clips on the same track without touching the master gain.
	Gain float64 `toml:"gain" validate:"min=-60,max=6"`
}

// ─────────────────────────────────────────────
//  Automation
// ─────────────────────────────────────────────

// AutomationPoint is one node on an automation curve: at beat position Beat, the
// target parameter should have the value Value. The engine linearly interpolates
// between consecutive points.
//
// Example TOML:
//
//	[[automation.point]]
//	beat  = 0
//	value = -24.0
type AutomationPoint struct {
	Beat  float64 `toml:"beat" validate:"min=0"`
	Value float64 `toml:"value"`
}

// Automation is a single parameter automated over time on a track.
//
// Target names which parameter to drive:
//   - "gain"        — track gain in dB
//   - "pan"         — track pan, -1..1
//   - "fx[N].key"   — an FX parameter, e.g. "fx[0].wet" or "fx[0].low_db"
//
// Example TOML (in a track file):
//
//	[[automation]]
//	target = "gain"
//
//	[[automation.point]]
//	beat  = 0
//	value = -24.0
//
//	[[automation.point]]
//	beat  = 16
//	value = 0.0
type Automation struct {
	// Target is the parameter to automate (see type doc for the syntax).
	Target string `toml:"target" validate:"required"`

	// Points are the curve nodes. They should be ordered by beat, but the
	// engine sorts them defensively. At least one point is required.
	Points []AutomationPoint `toml:"point" validate:"required,min=1"`
}

// ─────────────────────────────────────────────
//  Track
// ─────────────────────────────────────────────

// Track represents a single audio track in the project.
// Each track has its own gain, pan, fx chain, and list of clips.
// Tracks are defined in separate files (e.g. tracks/kick.toml) and
// referenced from project.toml via the layout.tracks array.
//
// Example TOML (tracks/kick.toml):
//
//	id   = "kick"
//	bus  = "drums"
//	gain = 0.0
//	pan  = 0.0
//
//	[[fx]]
//	type = "eq_3band"
//	...
//
//	[[clip]]
//	file  = "samples/kick.wav"
//	start = 0.0
//	end   = 16.0
type Track struct {
	// ID uniquely identifies this track within the project.
	// Other parts of the project (automation, buses) reference tracks by ID.
	ID string `toml:"id" validate:"required"`

	// Bus is the optional ID of a bus this track routes into.
	// If empty, the track routes directly to the master output.
	Bus string `toml:"bus"`

	// Gain is the track volume in dB. 0.0 = unity gain (no change).
	// Practical range is -60 (near silence) to +6 (slight boost).
	Gain float64 `toml:"gain" validate:"min=-60,max=6"`

	// Pan positions the track in the stereo field.
	// -1.0 = fully left, 0.0 = center, 1.0 = fully right.
	Pan float64 `toml:"pan" validate:"min=-1,max=1"`

	// Mute silences this track without removing it from the project.
	Mute bool `toml:"mute"`

	// Solo mutes all other tracks, useful for focusing on one track.
	Solo bool `toml:"solo"`

	// FX is the effects chain for this track.
	// Effects are applied in order: FX[0] → FX[1] → FX[2] → ...
	// Order matters — a compressor before a reverb sounds different
	// than a reverb before a compressor.
	FX []FX `toml:"fx"`

	// Clips are the audio regions placed on this track's timeline.
	Clips []Clip `toml:"clip"`

	// Automation lists parameters automated over time on this track (e.g. a
	// gain fade, a filter sweep). Empty means a static mix.
	Automation []Automation `toml:"automation"`

	// SourceFile is not from TOML — we set this in the loader so we know
	// which file this track was loaded from. Useful for error messages
	// ("error in tracks/kick.toml line 4") and for the file watcher.
	SourceFile string `toml:"-"`
}

// ─────────────────────────────────────────────
//  Bus
// ─────────────────────────────────────────────

// Bus is a sub-mix channel that one or more tracks can route into.
// Buses let you apply effects to a group of tracks at once — for example,
// routing all drums into a "drums" bus and applying a compressor there
// instead of on each individual drum track.
//
// Example TOML (buses/drums.toml):
//
//	id   = "drums"
//	gain = -1.5
//
//	[[fx]]
//	type      = "compressor"
//	threshold = -12.0
type Bus struct {
	// ID uniquely identifies this bus. Tracks reference buses by this ID.
	ID string `toml:"id" validate:"required"`

	// Gain is the bus output level in dB.
	Gain float64 `toml:"gain" validate:"min=-60,max=6"`

	// FX is the effects chain applied to the summed output of all tracks
	// that route into this bus.
	FX []FX `toml:"fx"`

	// SourceFile tracks which file this bus was loaded from.
	SourceFile string `toml:"-"`
}

// ─────────────────────────────────────────────
//  Master
// ─────────────────────────────────────────────

// Master is the final stage of the signal chain.
// All buses (and tracks not assigned to a bus) feed into the master.
// The master fx chain is applied last, before output or export.
//
// Example TOML (master.toml):
//
//	gain    = 0.0
//	limiter = true
//
//	[[fx]]
//	type    = "eq_3band"
//	low_db  = 1.5
type Master struct {
	// Gain is the master output level in dB.
	Gain float64 `toml:"gain" validate:"min=-60,max=6"`

	// Limiter enables a safety limiter on the master output.
	// This prevents digital clipping (values above 0 dBFS) which
	// sounds like a harsh crack/distortion.
	Limiter bool `toml:"limiter"`

	// FX is the master effects chain.
	FX []FX `toml:"fx"`

	// SourceFile tracks which file master was loaded from.
	SourceFile string `toml:"-"`
}

// ─────────────────────────────────────────────
//  Transport
// ─────────────────────────────────────────────

// Transport holds the timing and playback settings for the project.
// These affect how beat positions are converted to sample positions —
// changing BPM shifts the timing of every clip simultaneously.
type Transport struct {
	// BPM is beats per minute — the tempo of the project.
	// 120 BPM = 2 beats per second = common "house music" tempo.
	// 128 BPM = common techno/EDM tempo.
	BPM float64 `toml:"bpm" validate:"required,min=20,max=300"`

	// TimeSig is the time signature as a string like "4/4" or "3/4".
	// "4/4" means 4 beats per bar — the most common in western music.
	// This affects how the UI draws bar lines, not the audio engine directly.
	TimeSig string `toml:"time_sig" validate:"required"`

	// SampleRate is the number of audio samples per second.
	// 44100 Hz = CD quality. 48000 Hz = standard for video/professional audio.
	// This must match your audio interface's sample rate.
	SampleRate int `toml:"sample_rate" validate:"required,oneof=44100 48000 88200 96000"`

	// BitDepth is the number of bits per sample for internal processing.
	// 32 = float32 (recommended, maximum headroom for processing).
	// 24 = standard for export. 16 = CD quality export.
	BitDepth int `toml:"bit_depth" validate:"required,oneof=16 24 32"`
}

// ─────────────────────────────────────────────
//  Layout
// ─────────────────────────────────────────────

// Layout is the section of project.toml that lists which files to load.
// It acts as a manifest — the project root that ties everything together.
//
// Example TOML:
//
//	[layout]
//	master = "master.toml"
//	buses  = ["buses/drums.toml"]
//	tracks = ["tracks/kick.toml", "tracks/pad.toml"]
type Layout struct {
	// Master is the path to master.toml, relative to project.toml.
	Master string `toml:"master" validate:"required"`

	// Buses is the list of bus file paths to load.
	// Order doesn't matter — buses are identified by their ID field.
	Buses []string `toml:"buses"`

	// Tracks is the list of track file paths to load.
	// Order matters for the UI (top-to-bottom display) but not for audio.
	Tracks []string `toml:"tracks" validate:"required,min=1"`
}

// ─────────────────────────────────────────────
//  ProjectMeta
// ─────────────────────────────────────────────

// ProjectMeta holds general metadata about the project.
type ProjectMeta struct {
	Name    string `toml:"name" validate:"required"`
	Version string `toml:"version"`
	Author  string `toml:"author"`
}

// ─────────────────────────────────────────────
//  Project (root)
// ─────────────────────────────────────────────

// Project is the fully loaded, fully resolved project.
// After the loader runs, this struct contains everything — all tracks,
// all buses, master, and transport settings. Nothing is left as a file path.
// This is what the engine receives and works with.
type Project struct {
	// Meta holds name, version, author from [project] in project.toml.
	Meta ProjectMeta `toml:"project"`

	// Transport holds BPM, sample rate, etc. from [transport] in project.toml.
	Transport Transport `toml:"transport"`

	// Layout is the raw file manifest from [layout] in project.toml.
	// After loading, the actual content is in Tracks/Buses/Master below.
	Layout Layout `toml:"layout"`

	// Master is the loaded and parsed master.toml.
	// Populated by the loader — not present in project.toml directly.
	Master *Master

	// Buses is the list of loaded buses, in the order listed in layout.buses.
	Buses []*Bus

	// Tracks is the list of loaded tracks, in the order listed in layout.tracks.
	Tracks []*Track

	// RootDir is the directory containing project.toml.
	// All relative paths in track/bus/clip files are resolved against this.
	// Set by the loader, not present in any TOML file.
	RootDir string
}
