package lsp

import (
	"sort"
	"strings"
)

// What CodaW's files may contain, section by section: the keys, what their values are, and a line of
// documentation each. One table for completion (which keys fit here), hover (what is this key), and
// later warnings about keys the engine ignores. It mirrors internal/project (the structs and their
// validation) and internal/engine/fx.go (the effects and their parameters).

// fileKind is what a file is in the project, which decides the keys it may have.
type fileKind int

const (
	kindUnknown fileKind = iota
	kindProject
	kindTrack
	kindBus
	kindMaster
)

// key is one key a section may have.
type key struct {
	name string
	// kind of value: "number", "string", "bool" or "array".
	kind string
	doc  string
	// unit and valid range, as shown to the user ("dB, -60 to 6").
	detail string
	// values to suggest after "key = ", if the value is one of a fixed few.
	values []string
}

// sections lists, per kind of file, the keys of each section. The top of a file (before any header) is
// the section "". Arrays of tables are named without brackets: "clip", "automation.point".
var sections = map[fileKind]map[string][]key{
	kindProject: {
		"project": {
			{name: "name", kind: "string", doc: "The project's name."},
			{name: "version", kind: "string", doc: "A version of your choosing."},
			{name: "author", kind: "string", doc: "Who made it."},
		},
		"transport": {
			{name: "bpm", kind: "number", doc: "Tempo, in beats per minute.", detail: "20 to 300"},
			{name: "time_sig", kind: "string", doc: "Time signature: beats per bar / beat unit.", values: []string{`"4/4"`, `"3/4"`, `"6/8"`}},
			{name: "sample_rate", kind: "number", doc: "Sample rate the project renders at, in Hz.", values: []string{"44100", "48000", "88200", "96000"}},
			{name: "bit_depth", kind: "number", doc: "Bit depth of rendered files.", values: []string{"16", "24", "32"}},
		},
		"layout": {
			{name: "master", kind: "string", doc: "The master file, relative to project.toml."},
			{name: "buses", kind: "array", doc: "The bus files, in order."},
			{name: "tracks", kind: "array", doc: "The track files, in the order the views show them."},
		},
	},
	kindTrack: {
		"": {
			{name: "id", kind: "string", doc: "The track's name; unique in the project."},
			{name: "bus", kind: "string", doc: "The bus this track feeds. Without one, it goes straight to the master."},
			{name: "gain", kind: "number", doc: "The track's level.", detail: "dB, -60 to 6"},
			{name: "pan", kind: "number", doc: "Position in the stereo field.", detail: "-1 (left) to 1 (right)"},
			{name: "mute", kind: "bool", doc: "Silences the track.", values: []string{"true", "false"}},
			{name: "solo", kind: "bool", doc: "Plays only the soloed tracks.", values: []string{"true", "false"}},
		},
		"clip": clipKeys,
		"automation": {
			{name: "target", kind: "string", doc: "What the lane changes: `gain`, `pan`, or a parameter of one of the track's effects as `fx[N].param`, N counting the `[[fx]]` from 0."},
		},
		"automation.point": {
			{name: "beat", kind: "number", doc: "Where the point is.", detail: "beats, from 0"},
			{name: "value", kind: "number", doc: "The target's value there; in between, it changes in a straight line."},
		},
	},
	kindBus: {
		"": {
			{name: "id", kind: "string", doc: "The bus's name, which tracks send to with bus = \"…\"."},
			{name: "gain", kind: "number", doc: "The bus's level.", detail: "dB, -60 to 6"},
		},
	},
	kindMaster: {
		"": {
			{name: "gain", kind: "number", doc: "The master's level.", detail: "dB, -60 to 6"},
			{name: "limiter", kind: "bool", doc: "A limiter on the master, against clipping. (Not implemented in the engine yet.)", values: []string{"true", "false"}},
		},
	},
}

var clipKeys = []key{
	{name: "file", kind: "string", doc: "The audio file, relative to project.toml."},
	{name: "start", kind: "number", doc: "Where the clip starts.", detail: "beats"},
	{name: "end", kind: "number", doc: "Where the clip ends; after start.", detail: "beats"},
	{name: "offset", kind: "number", doc: "Where in the audio file the clip begins.", detail: "seconds, from 0"},
	{name: "loop", kind: "bool", doc: "Repeats the audio until the clip ends: from the offset once, then from the file's start.", values: []string{"true", "false"}},
	{name: "gain", kind: "number", doc: "The clip's level, on top of the track's.", detail: "dB, -60 to 6"},
}

// effects are the effect types the engine knows, with their parameters. Every kind of file but the
// project has [[fx]].
var effects = map[string]struct {
	doc    string
	params []key
}{
	"eq_3band": {
		doc: "A three-band equalizer: low shelf, peak, high shelf.",
		params: []key{
			{name: "low_hz", kind: "number", doc: "Low shelf frequency.", detail: "Hz, default 100"},
			{name: "low_db", kind: "number", doc: "Low shelf boost or cut.", detail: "dB, default 0"},
			{name: "mid_hz", kind: "number", doc: "Peak frequency.", detail: "Hz, default 1000"},
			{name: "mid_db", kind: "number", doc: "Peak boost or cut.", detail: "dB, default 0"},
			{name: "high_hz", kind: "number", doc: "High shelf frequency.", detail: "Hz, default 8000"},
			{name: "high_db", kind: "number", doc: "High shelf boost or cut.", detail: "dB, default 0"},
		},
	},
	"reverb": {
		doc: "Reverb (an echo approximation for now).",
		params: []key{
			{name: "room_size", kind: "number", doc: "Size of the room.", detail: "0 to 1, default 0.5"},
			{name: "wet", kind: "number", doc: "How much of the effect is heard.", detail: "0 to 1, default 0.3"},
		},
	},
	"compressor": {
		doc: "A compressor. (No DSP in the engine yet: the audio passes through unchanged.)",
		params: []key{
			{name: "threshold", kind: "number", doc: "Level above which it compresses.", detail: "dB"},
			{name: "ratio", kind: "number", doc: "How strongly: 4 means 4 dB over the threshold come out as 1."},
			{name: "attack_ms", kind: "number", doc: "How fast it reacts.", detail: "ms"},
			{name: "release_ms", kind: "number", doc: "How fast it lets go.", detail: "ms"},
			{name: "makeup_db", kind: "number", doc: "Level added after compressing.", detail: "dB"},
		},
	},
}

// fxTypeKey is the key every [[fx]] has, whatever its type.
var fxTypeKey = key{name: "type", kind: "string", doc: "Which effect: \"eq_3band\", \"reverb\" or \"compressor\"."}

// headers are the sections a kind of file may have, as typed: for completing a "[" line.
var headers = map[fileKind][]string{
	kindProject: {"[project]", "[transport]", "[layout]"},
	kindTrack:   {"[[clip]]", "[[fx]]", "[[automation]]", "[[automation.point]]"},
	kindBus:     {"[[fx]]"},
	kindMaster:  {"[[fx]]"},
}

// keysFor are the keys of a section in a kind of file; fxType is the type of the [[fx]] the cursor
// is in, for its parameters.
func keysFor(kind fileKind, section, fxType string) []key {
	if section == "fx" {
		keys := []key{fxTypeKey}
		if effect, ok := effects[fxType]; ok {
			keys = append(keys, effect.params...)
		}
		return keys
	}
	return sections[kind][section]
}

func findKey(keys []key, name string) (key, bool) {
	for _, k := range keys {
		if k.name == name {
			return k, true
		}
	}
	return key{}, false
}

var sectionDocs = map[string]string{
	"project":          "What the project is called and who made it.",
	"transport":        "Tempo and audio format.",
	"layout":           "The files that make up the project: the master, the buses, the tracks.",
	"clip":             "A piece of audio on the timeline: a file, and where it starts and ends in beats. To split a clip, write two that share a file.",
	"fx":               "An effect. Effects run in the order they are written; automation refers to them by position, as `fx[0]`, `fx[1]`, …",
	"automation":       "A lane that changes one value over time, between its `[[automation.point]]`s.",
	"automation.point": "A point of the `[[automation]]` above: a beat and the value there.",
}

// sectionOf finds a section among those a kind of file may have, and whether it is a list ([[x]]).
func sectionOf(kind fileKind, name string) (array, ok bool) {
	for _, header := range headers[kind] {
		if strings.Trim(header, "[]") == name {
			return strings.HasPrefix(header, "[["), true
		}
	}
	return false, false
}

func effectNames() []string {
	names := make([]string, 0, len(effects))
	for name := range effects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// keysOf are the keys a table of a document may have.
func keysOf(kind fileKind, t *table) []key {
	return keysFor(kind, t.name, t.text("type"))
}

// closest is the candidate a misspelled name most likely meant, or "".
func closest(name string, candidates []string) string {
	best, bestDistance := "", 3
	for _, candidate := range candidates {
		if d := distance(name, candidate); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	return best
}

func distance(a, b string) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		previous := row[0]
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := previous
			if a[i-1] != b[j-1] {
				cost = min(previous, row[j], row[j-1]) + 1
			}
			previous, row[j] = row[j], cost
		}
	}
	return row[len(b)]
}
