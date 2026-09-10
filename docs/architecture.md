# Architecture — options for the next engine

Planning notes, not a decision. Companion to `docs/plugins.md` (plugins are Go
functions) and `docs/roadmap.md` (what order to build in).

The one part treated as settled here: **malgo at the bottom for the device,
a Go mixer above it, and audio as `[]float32` we own.** Everything below is
about what sits between those two.

## Three shapes for the mixer

### A. Flat scheduler — a loop over the project

```go
for each block {
    zero the track buffers
    for each clip playing right now → read samples into its track buffer
    for each track → gain, pan, fx → sum into its bus buffer
    for each bus   → fx → sum into master
    master fx → out
}
```

- ✅ You can read the entire signal path in one function. No interfaces, no
  indirection, nothing hidden.
- ✅ Trivial to preallocate — every buffer is known at build time, so
  zero-allocation in the audio path is easy to reach *and easy to assert*.
- ✅ Matches the TOML schema exactly. The project model already *is*
  track → bus → master; the code would just say so.
- ✅ Easiest to debug: print any buffer at any stage.
- ❌ Routing is hardcoded to those three tiers. Sends, sidechains or parallel
  chains mean editing the loop rather than adding a node.
- ❌ Less satisfying if you like composable designs.

**Best when** the routing model is fixed and you want to move fast. That's
CodaW today.

### B. Streamer tree — everything is a source

```go
type Source interface {
    Read(buf []float32) (n int, err error)
}
```

Clips, tracks, buses and the master are all `Source`. Master pulls from buses,
buses pull from tracks, tracks pull from clips. The sink pulling the master is
a WAV writer, the device, or a test buffer.

- ✅ **Render and playback are the same code by construction** — the only
  difference is who pulls. Not a property you maintain; a property you get.
- ✅ Every node testable in isolation with a plain float slice.
- ✅ A plugin is a `Source` that wraps another `Source`. Very natural fit.
- ✅ Familiar shape (it's `io.Reader`), and roughly what `gopxl/beep` does, so
  there's prior art to crib from.
- ❌ Short reads and end-of-stream handling get subtle when nodes must stay in
  lockstep on the timeline.
- ❌ One output per node, so sends and sidechains are awkward — the same
  ceiling as A, reached differently.
- ❌ Interface dispatch and buffer handoffs make "no allocations" something you
  have to watch rather than something obvious.

**Best when** you want the render/playback equivalence guaranteed structurally
and don't mind a layer of abstraction.

### C. Generic node graph — nodes, ports, topological order

Nodes with input/output ports, sorted once into a processing order, each
writing into port buffers. What miniaudio was doing for you.

- ✅ Handles everything: sends, sidechain compression, parallel chains,
  feedback loops, per-node latency compensation.
- ✅ The shape every mature DAW converges on, so you'd never hit a wall.
- ❌ It is the thing you are currently removing *because it's opaque*. Building
  your own version means owning topological sort, cycle detection, port buffer
  allocation and latency compensation.
- ❌ Large up-front cost for routing flexibility no CodaW project can express
  yet — the TOML schema has no concept of a send.

**Best when** sends and sidechains are real requirements. They aren't yet.

### Recommendation

**Start with A.** It's the least code, the most debuggable, and it mirrors the
file format. If sends become real, A grows into C — and the growth is
mechanical, because by then you'll know exactly which routings you need.

B is the tempting middle. Its one genuine advantage over A — render ≡ playback
— can be had in A for free by writing a single `mixBlock()` that both the WAV
writer and the device callback call. That's the property that matters; the
interface is optional.

## The cross-cutting decision: snapshot vs. events

Independent of A/B/C, and probably higher-leverage than the choice above.

**Today:** the store emits typed events (`EventFXParamChanged`, …), and the
engine mutates the live graph in place. `state/events.go` and
`state/mutations.go` are ~480 lines, and every new parameter costs a new event
type, a mutation, and an engine handler. That grows linearly, forever.

**Alternative:** the project is an **immutable snapshot swapped atomically**.

```go
var current atomic.Pointer[Session]   // watcher stores, audio thread loads

// audio side, once per block:
s := current.Load()
```

- No locks on the audio path at all — which matters, because a mutex shared
  with the watcher goroutine is a dropout waiting to happen.
- Adding a parameter costs *nothing*: it's a struct field the mixer reads.
- Reloads become "parse the TOML, build a Session, swap the pointer."
- Rebuild cost is irrelevant — it's a few small TOML files, not a DAW project.

**The catch, and it's the important one: separate the score from the
performance.** The snapshot holds project data. Runtime state — playhead,
filter memory, reverb tails, per-clip read positions, envelope state — must
live *outside* it and survive the swap, keyed by stable IDs. Conflate the two
and every file save resets filters and clicks. Get this boundary right and
hot-reload stays the project's best feature.

## A package layout for option A

```
internal/
  project/   TOML → Project                          (unchanged)
  state/     store + watcher plumbing                (shrinks a lot)
  sample/    decode → []float32, cache, resample     ← new
  dsp/       biquad, gain, pan, compressor, limiter  ← new
  mix/       clips → tracks → buses → master         ← new
  plugin/    registry of Go effect functions         ← new
  audio/     malgo device + ring buffer + sinks      (rewritten, no C)
  serve/     JSON-lines control channel              (unchanged)
```

`sample/` earning its own package is deliberate: working with audio files is
the thing that matters most here, and it deserves a home where trimming,
normalizing, waveform extraction and recorded-take handling all live together,
independent of playback.

## Watch-outs

### Audio-thread rules

- **Never allocate in the mix path.** Every `make()` is GC pressure and GC is
  a click. Preallocate at build time — and *assert* it: `testing.AllocsPerRun`
  on `mixBlock()` can be pinned at zero in a normal Go test.
- **Never lock.** Use the atomic snapshot swap above.
- **Never touch the filesystem.** Recording writes go through a ring buffer to
  a writer goroutine, never from the callback.
- **Keep a ring buffer between your mixer and malgo's callback.** It decouples
  a GC pause from the device at the cost of latency, which CodaW can afford.

### DSP gotchas that will actually bite

- **Denormals.** Reverb and filter tails decay toward zero, hit denormal
  floats, and can cost 10–100× CPU on x86 — sudden crackle after a loud
  passage. Go can't set the FTZ flag, so flush tiny values to zero yourself in
  feedback paths.
- **Zipper noise.** Changing gain, pan or filter coefficients abruptly between
  blocks clicks audibly. Ramp per-block or one-pole smooth every control value.
  This lands the moment TOML hot-reload touches a gain — i.e. immediately.
- **Clip-boundary clicks.** A clip that starts or ends mid-waveform pops. A
  ~5 ms fade at both edges of every clip fixes it and costs nothing. Trimming
  and splitting make this unavoidable.
- **Headroom.** Summed tracks exceed ±1.0 easily. Decide now whether the
  master hard-clips, soft-clips, or limits — `master.limiter` currently
  promises protection it doesn't provide.

### Correctness and reproducibility

- **One frame counter is the truth.** Derive beats from frames; never
  accumulate a float position per block — it drifts.
- **Don't drive automation from a wall clock.** `automationLoop` currently
  ticks every 20 ms live, while `Render` samples lanes per 4096-frame chunk.
  Two implementations, two resolutions, so live and rendered output already
  differ, and both stair-step a fast fade. A shared `mixBlock()` that samples
  automation per block fixes this by construction. Sample-accurate ramps
  within a block are a later refinement.
- **Sample rates.** Device rate, project rate and file rate are three
  different numbers. Pick one internal rate (the project's), resample files
  once at load where CPU doesn't matter, and resample only at the device edge.
  Today `transport` quietly uses the *device* rate while the project declares
  its own.
- **Test with golden renders,** compared with a tolerance rather than byte
  equality — math functions and resamplers vary across platforms.

### Recording

- **Use duplex, not two devices**, if input must line up with playback —
  separate capture and playback clocks drift apart.
- **Decide what a take is** before writing code: where the WAV lands, its bit
  depth, and how the clip gets written into the track TOML. Recording is the
  first feature where the engine *writes* project data, which the file-plane
  rule in `docs/ipc.md` currently forbids. That rule needs an explicit
  exception, written down.
- **Monitoring changes the latency budget.** Playback-only can afford big
  buffers; hearing yourself through the engine cannot. If monitoring is ever
  wanted, decide it early — it constrains the whole design.

## Decide once, early

These change how every existing project sounds if you revisit them later:

| Decision | Suggested default |
|---|---|
| Internal sample format | `float32`, interleaved |
| Internal sample rate | the project's, resample at the edges |
| Pan law | constant power (−3 dB centre) |
| Gain law | dB → linear, `10^(dB/20)` |
| Block size | 1024 frames (~21 ms @ 48 k), configurable |
| Clip edge fade | 5 ms in/out, always |
| Master safety | soft limiter, on by default |
