# Plugins — design notes

Status: **decided, not implemented.**
Decision: **a plugin is a Go function. The compile step is `go build`.**

## The shape

Writing a plugin should be something a person who knows a little Go can do in
one sitting, by copying a file and changing the maths. So a plugin is one Go
file that registers a function:

```go
// plugins/tape_sat.go
package plugins

import "math"

func init() {
	Register(Plugin{
		ID:   "tape_sat",
		Name: "Tape Saturator",
		Params: []Param{
			{Key: "drive", Default: 0.3, Min: 0, Max: 1},
		},
		// Called once per sample. Return the new value. That's the whole API.
		Sample: func(x float32, p Params) float32 {
			drive := 1 + p.Get("drive")*24
			return float32(math.Tanh(float64(x) * drive))
		},
	})
}
```

Drop the file in, `go build`, and `tape_sat` exists. No ABI, no runtime, no
sandbox, no cross-compilation matrix, no new dependency.

Effects that need memory between samples (filters, delays, compressors)
implement `Block` instead, which gets the whole buffer plus a per-instance
state struct. `Sample` is the easy door; `Block` is the one with room in it.

## The TOML side does not change

This is the part that stays exactly as it is today — params live in TOML, the
watcher hot-reloads them, the mixer UI drives them:

```toml
[[fx]]
type  = "tape_sat"
drive = 0.6
```

Automation already works too, because the target syntax is index-based and
`internal/engine/automation.go` doesn't care what kind of effect `fx[0]` is:

```toml
[[automation]]
target = "fx[0].drive"
```

So a plugin author writes *only* the maths. Interactivity, hot-reload, knobs
and automation come from the machinery that already exists.

The `Params` declaration is the one genuinely new piece of value: today
`vscode-extension/webviews/lib/fxParams.ts` guesses knob ranges from name
suffixes (`_hz` → 20–20000, log). With declared params the engine can hand the
extension real ranges and labels, so a new plugin gets correct knobs with no
UI work.

## The one piece of plumbing this needs

Every effect today is a stock miniaudio node wired up in
`internal/audio/effect.go`. To run *our own* function on audio we need one
custom miniaudio node whose process callback calls into Go. That's a single
cgo file, and it's the same thing the compressor (currently passes audio
through untouched) and the master limiter (currently just logs a warning)
need anyway. Build it once, get all three.

Two rules the plugin API has to enforce, both easy to document:

1. **Don't allocate in the processing function.** Allocate state up front.
2. **Don't block** — no file reads, no channels, no locks.

These are the normal rules of audio programming, not Go-specific.

## Distribution: copy the file, rebuild

For now, sharing a plugin means sharing a `.go` file — a gist, a PR, a repo you
copy from. It gets compiled into your `codaw` binary.

That is a real limitation and worth stating plainly: you cannot install a
plugin without rebuilding, so a non-technical user can't add one to a release
binary. It is also the *only* option that costs nothing to build and nothing
to learn, and it can be revisited once plugins exist and someone actually
wants to distribute one.

## What we're deliberately not doing

Each of these was considered and is a bigger project than the thing it buys:

- **WebAssembly plugins.** Sandboxed, distributable as one cross-platform
  artifact, any source language. Also: a wasm runtime in the audio path, an
  ABI to design and version, a memory-marshalling story, and a `plugin build`
  command. Right answer eventually; wrong answer first.
- **Native `.dylib`/`.so` plugins** (Odin, C, Rust). Fast and simple to load,
  but hands every plugin author a per-platform build matrix and every user an
  unsandboxed binary from a stranger.
- **Lua or another embedded interpreter.** No compile step, but an interpreted
  per-sample loop is far too slow, and the interpreter's garbage collector
  running on the audio thread is a click in the output.
- **A declarative DSP graph in TOML** (effects built from primitives, no code).
  Genuinely elegant and needs no toolchain — but a compile step turned out to
  be acceptable, and that was the only thing this bought.
- **Rust.** Excellent for DSP and completely at odds with "someone who knows a
  little Go can write one in an afternoon."

The Go decision is reversible in the direction that matters: `Params` +
`Sample`/`Block` is a small enough surface that a wasm backend could implement
the same shape later without changing how plugins are declared or used.
