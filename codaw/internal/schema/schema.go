// Package schema says what CodaW's project files may contain: every section, every key, its type,
// unit, range and default, and every effect with its parameters. It is the one place that is
// written down. Validation, the language server and the engine's defaults read it, and
// `codaw schema` prints it for the editor.
//
// It imports nothing of CodaW's, so every other package can import it.
package schema

import (
	"fmt"
	"strconv"
	"strings"
)

type Kind string

const (
	Number Kind = "number"
	String Kind = "string"
	Bool   Kind = "bool"
	List   Kind = "list"
)

// Key is one key of a section, or one parameter of an effect.
type Key struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	Unit string `json:"unit,omitempty"`
	// The valid range of a number; nil where there is no bound.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// The only values a number may have.
	OneOf []float64 `json:"oneOf,omitempty"`
	// Values worth suggesting; others are allowed.
	Suggest []string `json:"suggest,omitempty"`
	// What the engine uses when the key is missing.
	Default  any    `json:"default,omitempty"`
	Required bool   `json:"required,omitempty"`
	Doc      string `json:"doc"`
}

// Section is a table of a file: "[transport]", "[[clip]]". The one named "" is the top of a file,
// before any header.
type Section struct {
	Name string `json:"name"`
	List bool   `json:"list"` // written [[name]], any number of times
	Doc  string `json:"doc,omitempty"`
	Keys []Key  `json:"keys"`
}

type FileKind string

const (
	ProjectFile FileKind = "project"
	TrackFile   FileKind = "track"
	BusFile     FileKind = "bus"
	MasterFile  FileKind = "master"
)

// File is one kind of file, with its sections in the order they are usually written.
type File struct {
	Kind     FileKind  `json:"kind"`
	Sections []Section `json:"sections"`
}

// Effect is one type of [[fx]].
type Effect struct {
	Name   string `json:"name"`
	Doc    string `json:"doc"`
	Params []Key  `json:"params"`
}

func (f File) Section(name string) (Section, bool) {
	for _, s := range f.Sections {
		if s.Name == name {
			return s, true
		}
	}
	return Section{}, false
}

func (s Section) Key(name string) (Key, bool) {
	return find(s.Keys, name)
}

func (e Effect) Param(name string) (Key, bool) {
	return find(e.Params, name)
}

func find(keys []Key, name string) (Key, bool) {
	for _, k := range keys {
		if k.Name == name {
			return k, true
		}
	}
	return Key{}, false
}

func FileOf(kind FileKind) (File, bool) {
	for _, f := range Files {
		if f.Kind == kind {
			return f, true
		}
	}
	return File{}, false
}

func EffectNamed(name string) (Effect, bool) {
	for _, e := range Effects {
		if e.Name == name {
			return e, true
		}
	}
	return Effect{}, false
}

// Check says what is wrong with a number for this key, or "" if nothing is.
func (k Key) Check(value float64) string {
	got := ", got " + Format(value)
	switch {
	case len(k.OneOf) > 0:
		for _, allowed := range k.OneOf {
			if value == allowed {
				return ""
			}
		}
		names := make([]string, len(k.OneOf))
		for i, allowed := range k.OneOf {
			names[i] = Format(allowed)
		}
		return "must be " + strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1] + got
	case k.Min != nil && k.Max != nil && (value < *k.Min || value > *k.Max):
		return strings.TrimSpace(fmt.Sprintf("must be between %s and %s %s", Format(*k.Min), Format(*k.Max), k.Unit)) + got
	case k.Min != nil && value < *k.Min:
		return "must be >= " + Format(*k.Min) + got
	case k.Max != nil && value > *k.Max:
		return "must be <= " + Format(*k.Max) + got
	}
	return ""
}

// Resolve gives every parameter of an effect a value: the file's where it has a number, the
// default otherwise.
func (e Effect) Resolve(params map[string]any) map[string]float64 {
	out := make(map[string]float64, len(e.Params))
	for _, k := range e.Params {
		if value, ok := AsNumber(params[k.Name]); ok {
			out[k.Name] = value
		} else if value, ok := AsNumber(k.Default); ok {
			out[k.Name] = value
		}
	}
	return out
}

// AsNumber reads a number as TOML decodes it: integers as int64, floats as float64.
func AsNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}

// Format writes a number without trailing zeros: 6, -0.5, 44100.
func Format(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
