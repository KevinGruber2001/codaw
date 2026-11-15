// Package dsp is CodaW's effects: the [[fx]] types a project can use, as plain Go.
//
// What an effect's parameters are (names, ranges, defaults, docs) is written down once, in
// internal/schema. This package is how each effect sounds. A test keeps the two in step: every
// effect in the schema has an implementation here, and accepts every parameter the schema gives it.
//
// Effects run on the mixer's goroutine, a block at a time. Process must not allocate, lock or log:
// everything it needs is allocated when the effect is made.
package dsp

import (
	"fmt"
	"sort"

	"github.com/KevinGruber2001/codaw/internal/schema"
)

// Effect processes audio in place: interleaved stereo float32, frames frames of it.
//
// An effect keeps its state (filter memory, envelopes, delay lines) for as long as it lives, and the
// mixer keeps it alive across project reloads. A saved TOML file therefore only changes parameters,
// through SetParam; it never resets the state, which would click.
type Effect interface {
	// SetParam sets a parameter's target. The effect moves to it over a few milliseconds rather than
	// jumping, so a change while playing does not zipper. False for a parameter it does not have.
	SetParam(name string, value float64) bool
	Process(buf []float32, frames int)
}

// Factory makes an effect for a sample rate. The mixer then sets every parameter, so a factory need
// not know the defaults: they come from the schema.
type Factory func(rate int) Effect

var factories = map[string]Factory{}

// Register adds an effect type, under the name `type = "…"` gives it. Called from init functions.
func Register(name string, factory Factory) {
	if _, taken := factories[name]; taken {
		panic("dsp: effect " + name + " registered twice")
	}
	factories[name] = factory
}

// Names are the registered effect types, sorted.
func Names() []string {
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// New makes an effect of a type, with its parameters: the file's values, the schema's defaults for
// the rest. Parameters start at their values; only later changes are smoothed.
func New(fxType string, rate int, params map[string]any) (Effect, error) {
	factory, ok := factories[fxType]
	if !ok {
		return nil, fmt.Errorf("dsp: unknown effect %q", fxType)
	}
	e := factory(rate)
	if declared, ok := schema.EffectNamed(fxType); ok {
		for name, value := range declared.Resolve(params) {
			e.SetParam(name, value)
		}
	}
	if s, ok := e.(interface{ snap() }); ok {
		s.snap()
	}
	return e, nil
}
