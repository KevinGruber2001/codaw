# Plugins — design notes

Status: **decided, not implemented** — see issues #4 (built-ins) and #7 (user plugins).

Two kinds of effect, one registry.

## Built-in effects are Go functions

The EQ, compressor, limiter and reverb ship with CodaW and are plain Go:

```go
type Effect func(buf []float32, frames, channels int)
```

Native speed, no abstraction, and they run on every track of every project.

## User plugins are Lua files in the project

A plugin belongs to a **project**, not to a release. Someone making a song adds an
effect to their own project without rebuilding CodaW and without waiting for one:

```
my-song/
  project.toml
  plugins/
    tape_sat.lua      <- committed with the song
```

```lua
params { drive = { default = 0.3, min = 0, max = 1 } }

function process(block, p)
  local d = 1 + p.drive * 24
  for i = 1, #block do
    block[i] = tanh(block[i] * d)
  end
end
```

Save the file, hear it. No compile step, no toolchain, no restart — the watcher
already reloads on save, and a `.lua` file is just another file in the project.

That is the argument in one line: CodaW's premise is *your project is plain text
you can edit and immediately hear*, and Lua is the only option that extends that
promise to effects rather than carving out an exception to it.

Both kinds register into the same registry, so the mixer, the automation targets,
validation and the mixer UI are written once and can't tell them apart.

## Why this is viable now

An earlier draft of this document rejected a scripting language because *"an
interpreter's garbage collector on the audio thread is a click in the output."*
The ring buffer in the new engine removes that objection: the mixer runs on an
ordinary goroutine and the device callback only copies bytes out of a ring, so a
slow plugin eats buffer headroom instead of glitching the device. It has to be
persistently too slow to be audible at all.

The API hands Lua a whole **block**, not one sample — crossing the Go↔Lua boundary
is the expensive part, so it happens once per ~1024 frames rather than 96,000
times a second.

## What we're deliberately not doing

- **WebAssembly.** Technically stronger — sandboxed, one artifact per platform,
  any source language, near-native speed. It loses on the requirements that matter
  here: the author needs a toolchain before writing a line, the artifact is a
  binary so "your song is plain text" breaks, and the host needs an ABI plus
  linear-memory marshalling. It is the documented escape hatch if Lua benchmarks
  badly, and because it would register into the same registry, switching later
  changes how plugins are *written*, not how they're used.
- **Go's `plugin` package.** No Windows, and it demands the exact same Go version
  and dependency set as the binary. Disqualified, not disfavoured.
- **Native `.so` / `.dylib`.** A per-platform build matrix for every author, an
  unsandboxed binary from a stranger for every user, and it needs cgo back.
- **Compiling plugins into the binary.** Simplest of all, and rejected outright:
  it makes a plugin part of a *release* instead of part of a *project*.
- **A declarative DSP graph in TOML.** Still the simplest thing that could work,
  and the honest fallback if Lua disappoints — but you could only combine
  primitives we ship, never write a new algorithm.
