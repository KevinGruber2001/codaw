# Plugins — design notes

Status: **not implemented — this is the decision doc.**
Decision: **three planes, not one language.** Declarative DSP patches now,
Wasm for real code later, subprocess tools for everything non-realtime.
Native `.dylib` plugins are an explicit non-goal.

## What "plugin as code" has to mean here

CodaW's whole premise is in the README: *the session is code you can version,
diff, and hand-edit.* A plugin ecosystem that ships opaque signed binaries
would undo that in one move — the moment `type = "some_comp"` resolves to a
blob nobody can read, the project stops being self-describing and starts
rotting the way VST projects rot. So the bar for anything we adopt is:

1. **Readable.** A plugin's source lives in the repo next to the TOML that
   uses it. `git diff` on a plugin is as meaningful as `git diff` on a track.
2. **Reproducible.** `codaw render` on a fresh checkout in five years, on a
   machine that isn't this one, produces the same WAV. That means vendored,
   hashed, and architecture-independent.
3. **Safe to install from a stranger.** Installing a plugin from GitHub must
   not be equivalent to running their shell script.
4. **Realtime-honest.** Whatever runs inside the audio callback obeys the
   callback's rules — no allocation, no locks, no syscalls, no GC pause. At
   48 kHz with a 480-frame buffer that budget is 10 ms, every 10 ms, forever.

Requirement 4 is the one that kills most candidates, and requirement 3 is the
one that kills the rest.

## What a plugin actually is (three different things)

We have been saying "plugin" for three unrelated jobs, which is why picking
"a language" felt impossible — they have opposite constraints:

| Plane | Job | Runs | Constraint that dominates |
|---|---|---|---|
| **FX** | process audio (comp, limiter, saturation, filters) | audio thread, every buffer | realtime safety |
| **Generator** | produce audio from notes/params (synths) | audio thread, every buffer | realtime safety + CodaW has no note data yet |
| **Tool** | write TOML (drum-pattern generator, arranger, gain-stager, importer) | offline, on demand | none, really |

Tools have no realtime constraint at all, so holding them to the FX answer
costs us an entire ecosystem for no reason. Generators need note data that
doesn't exist yet, so they're gated on the arrangement work, not on this
decision. **The only genuinely hard question is FX.**

## Alternatives considered (the FX plane)

### A. Lua (embedded interpreter)

- ✅ Tiny, embeddable, hot-reloadable, a language non-programmers can learn
- ✅ `gopher-lua` is pure Go — no extra cgo, no toolchain for the user
- ❌ **A per-sample loop in interpreted Lua is 50–100× too slow.** 48 000
  iterations per second per effect, through a bytecode VM, with boxed numbers
- ❌ LuaJIT closes most of that gap but is frozen at 5.1, needs cgo, and is
  awkward on macOS arm64
- ❌ Either way there is a **garbage collector on the audio thread**. An
  incremental GC step is still a step, and a step in the callback is a click
  in the output. This is disqualifying, not tunable.

Verdict: **wrong for FX, genuinely good for Tools.** See the Tool plane below.

### B. Odin (compiled to a native shared library)

- ✅ No GC, manual memory, C ABI, compiles to fast scalar and SIMD code —
  it is a legitimately excellent DSP language and would sit happily beside
  the existing cgo layer
- ❌ The artifact is a `.dylib` / `.so` / `.dll`. Now every plugin author
  owns a **build matrix** (macOS arm64 + x64, Linux, Windows), and every
  plugin consumer either trusts prebuilt binaries or installs the Odin
  toolchain. That is exactly the VST distribution model we're trying not to
  inherit.
- ❌ `dlopen` is **unsandboxed by construction**. Requirement 3 is gone: a
  plugin can read your filesystem, open sockets, and crash the engine with
  a null deref that we cannot catch.
- ❌ A stable C ABI becomes ours to maintain forever, across engine versions.
- ❌ A prebuilt `.dylib` in a song repo breaks requirement 1 and, on a new
  architecture, requirement 2.

Verdict: **not as the ecosystem substrate.** But Odin is not out — see the
Wasm decision, where it becomes one of the source languages you can use.

### C. WebAssembly (plugin is a `.wasm` module)

- ✅ **One artifact runs everywhere** — macOS/Linux/Windows, x64/arm64. No
  build matrix, ever. This alone is worth a lot to a two-person project.
- ✅ **Sandboxed by construction.** A module that imports nothing cannot
  touch the filesystem, the network, or the clock. We can *verify* that at
  load time by inspecting the import section, and refuse anything that asks
  for more. Requirement 3 satisfied by a check, not by trust.
- ✅ **Language-agnostic**: Rust, Zig, C, **Odin** (`-target:freestanding_wasm32`),
  AssemblyScript, and Faust all emit wasm. The ecosystem question stops being
  "which language do we impose" and becomes "which languages do we document".
- ✅ Deterministic float semantics → offline render matches realtime, and a
  render in 2031 matches a render today.
- ✅ Source + built module both vendor into the repo, so requirement 1 holds:
  the `.wasm` is a build artifact next to readable source, like a lockfile.
- ⚠️ Needs an **AOT/JIT runtime, not an interpreter** — an interpreted wasm
  loop is Lua all over again.
- ⚠️ The runtime must be called **from C, not from Go**. A cgo callback into
  Go on the audio thread can land on a GC assist; that reintroduces exactly
  the pause we rejected Lua for. WAMR (or wasm2c output) compiles into
  `bridge.c` beside miniaudio and keeps the callback a pure C call.
- ❌ Real implementation cost: a custom `ma_node`, a runtime dependency, an
  ABI, a param-marshalling story, and a `codaw plugin build` command.

Verdict: **the right long-term substrate, too big to be step one.**

### D. Declarative patch — the effect *is* TOML

An effect defined as a small graph of built-in DSP primitives (biquad, delay,
gain, waveshaper, envelope follower), with control-rate expressions over its
parameters. A modular-synth patch, in the project's own file format.

- ✅ **Zero toolchain.** No compiler, no runtime, no build step. Write a
  file, hear it — the existing watcher already hot-reloads it for free.
- ✅ **Safe and reproducible by construction.** There is no code, only a
  bounded instruction set we implement. Nothing to sandbox.
- ✅ Diffable and readable in the strongest possible sense — it *is* the
  project's own format, so requirement 1 is trivially met.
- ✅ Fast enough: primitives process a **whole block** in native code;
  parameter expressions evaluate **once per block**, not per sample. The
  interpretation overhead is per-block, so it vanishes.
- ✅ Lands a real ecosystem in weeks, not months: saturators, filter banks,
  chorus, sidechain-flavoured ducking, multiband splits.
- ❌ Hard expressiveness ceiling. You can wire primitives; you cannot write
  a novel algorithm, an FFT, or a convolution reverb.
- ❌ Every new capability is a change to *our* primitive set, i.e. a release.

Verdict: **the right step one** — and it stays useful after Wasm ships,
because most effects people actually want are patches, not algorithms.

### E. Faust

A functional DSP language purpose-built for this, with a large existing
effect library, compiling to C or wasm.

- ✅ Enormous head start on library content
- ❌ Not a host mechanism — it's a *source* language. It compiles to the
  artifacts options C and D produce.

Verdict: **not a competing option; a supported authoring path once Wasm
lands.** "Write it in Faust, ship the wasm" is a great story to document.

### F. Out-of-process plugins over a pipe

- ❌ Adds a process hop's latency to every audio buffer. Dead on arrival
  for FX.
- ✅ **Perfect for Tools**, which have no latency budget at all — and
  `docs/ipc.md` already establishes the JSON-lines conventions to reuse.

Verdict: **the Tool plane's answer.**

## Decision

**Phase 1 — Declarative patches (option D).** An FX plugin is a directory
with a `plugin.toml`: a manifest of parameters plus a chain of built-in DSP
primitives. Ships the ecosystem, the manifest format, the resolver, the
lockfile, and the generic UI — all the parts that are *not* the DSP runtime —
with no new toolchain for anyone.

**Phase 2 — Wasm (option C).** When someone hits the ceiling (and they will:
convolution, FFT, anything with real state machines), the same manifest gains
`impl = "plugin.wasm"`. **Because the manifest, the parameter model, the
resolver, the lockfile and the UI are identical, Phase 2 is additive** — it
swaps what sits behind one node type. Odin, Rust, Zig, C and Faust all become
first-class ways to write one.

**Phase 0, in parallel — Tool plugins (option F).** No realtime constraints,
no ABI risk, and the highest wow-per-hour in the whole project: a script that
writes TOML into your song. Subprocess with a JSON-lines contract, so it can
be written in *any* language — including Lua, which is where Lua actually
belongs here.

**Explicit non-goal: native `dlopen` plugins.** Not Odin, not C, not Rust.
The moment we accept an unsandboxed binary from a third party we lose the
safety story and the reproducibility story together. First-party DSP written
straight into `internal/audio` in C/cgo is a different thing and stays fine.

To answer the original question directly: **not Lua and not Odin as "the
plugin language" — but Lua is the natural fit for Tools, and Odin is a
first-class way to author a Phase-2 Wasm effect.**

## Sketch — a Phase 1 plugin

`plugins/tape_sat/plugin.toml`

```toml
[plugin]
id      = "tape_sat"
name    = "Tape Saturator"
version = "0.1.0"
kind    = "fx"
abi     = 1

[[param]]
key     = "drive"
label   = "Drive"
default = 0.3
min     = 0.0
max     = 1.0

[[param]]
key     = "tone_hz"
label   = "Tone"
default = 6000
min     = 500
max     = 18000
unit    = "Hz"
log     = true
smooth_ms = 20        # de-zipper on change and under automation

# The DSP: primitives in series. Values starting with "=" are control-rate
# expressions over the params above, evaluated once per block.
[[node]]
op = "gain"
db = "= drive * 24"

[[node]]
op = "tanh"

[[node]]
op = "lowpass"
hz = "= tone_hz"
q  = 0.707

[[node]]
op = "gain"
db = "= drive * -18"   # keep unity-ish output as drive rises
```

Used in a track exactly like a built-in — the call site doesn't change:

```toml
[[fx]]
type    = "tape_sat"
drive   = 0.6
tone_hz = 9000
```

And automated exactly like a built-in, because the target syntax is already
index-based (`internal/engine/automation.go` parses `fx[N].key`):

```toml
[[automation]]
target = "fx[0].drive"
```

### Primitive set, v1

`gain`, `pan`, `biquad` (`lowpass`/`highpass`/`bandpass`/`peak`/`loshelf`/`hishelf`),
`delay`, `tanh`, `softclip`, `hardclip`, `dcblock`, `envfollow`, `compressor`,
`mix` (dry/wet on the whole chain). Parallel branches are v1.1 — a linear
chain plus a global dry/wet covers most of what people build first.

The expression language is deliberately tiny: `+ - * / ( )`, the param names,
numeric literals, and `min`/`max`/`clamp`/`db2lin`/`lin2db`. It is parsed once
at load into a small stack program and evaluated per block. No loops, no
branches, no state — it cannot be slow and it cannot fail at runtime.

## Sketch — the Phase 2 Wasm ABI

Same manifest, one extra key:

```toml
[plugin]
id   = "conv_verb"
kind = "fx"
abi  = 1
impl = "conv_verb.wasm"     # instead of [[node]] entries
source = "src/lib.rs"       # for humans and for `codaw plugin build`
```

Exports the module must provide, and nothing else matters:

```
codaw_abi_version() -> i32
codaw_init(sample_rate: i32, max_block: i32, channels: i32) -> i32   // returns state ptr
codaw_param(state: i32, index: i32, value: f32)
codaw_process(state: i32, in_ptr: i32, out_ptr: i32, frames: i32)
codaw_reset(state: i32)
```

Ground rules, all enforceable at load:

1. **Zero imports.** The module may import nothing at all. No WASI, no clock,
   no host functions. That is the sandbox proof, and it also guarantees no
   syscall can occur inside `codaw_process`.
2. **All memory up front.** `codaw_init` is the only place a plugin may grow
   linear memory; `codaw_process` must not. The host rejects a module whose
   process path can call `memory.grow`.
3. **AOT-compile at load, in C.** The runtime lives in the cgo layer next to
   miniaudio; the audio callback calls a compiled function pointer and never
   crosses back into Go.
4. **Deterministic.** No wall clock, no unseeded randomness — otherwise
   offline render stops matching realtime playback, and requirement 2 dies.
5. **Buffer in, buffer out, in place-ish.** Interleaved float32, at most
   `max_block` frames, in a region the host owns.

## Distribution — plugins are vendored, hashed, and committed

The distribution model *is* the "as code" part, and it is the same one Go
modules and lockfiles taught everyone:

```
my-song/
  project.toml
  codaw.lock             # every plugin id → version → sha256
  plugins/
    tape_sat/            # vendored, committed with the song
      plugin.toml
```

- `codaw plugin add github.com/user/tape-sat@v0.2.0` fetches, verifies, and
  vendors into `plugins/`. There is no registry service — a plugin is a git
  repo containing a `plugin.toml`. An index repo can come later if anyone
  wants search.
- `codaw.lock` pins content hashes. `codaw plugin verify` fails loudly if a
  vendored plugin no longer matches its hash.
- **Vendored plugins are committed with the song.** A fresh clone renders
  identically with no network access. This is the feature that a VST-based
  DAW structurally cannot offer, and it's worth saying out loud in the README.
- Name resolution: built-ins (`reverb`, `eq_3band`, `compressor`) win on a
  collision; project plugins resolve from `plugins/<id>/`. If collisions ever
  become a real problem, `type = "@user/tape_sat"` is the escape hatch —
  don't build it before then.

## What this buys the UI for free

`vscode-extension/webviews/lib/fxParams.ts` currently guesses parameter
ranges from name suffixes (`_hz` → 20–20000 Hz, log) because "the TOML schema
is intentionally open". That heuristic is a stand-in for a manifest that
doesn't exist yet. Once `[[param]]` exists, the extension asks the engine for
the plugin manifests (one new `serve` command, `{"type":"plugins"}`) and
renders real labels, ranges, units and log scaling for **every** plugin,
built-ins included — with the suffix heuristic demoted to the fallback for
hand-written params. Third-party effects get correct knobs on day one without
anyone shipping UI code.

The same manifest is what `codaw validate` needs to stop accepting typos:
today `validateFXChain` only checks that `type` is non-empty, and
`buildEffect` logs "unknown fx — skipping" at runtime. With manifests, an
unknown `type` or an out-of-range param becomes a load-time error with a file
and line, which is the standard the rest of the loader already holds itself to.

## The dependency nobody should skip

Phase 1 needs a **custom miniaudio node with a C process callback** — the
first line of real DSP CodaW will own, since today every effect is a stock
miniaudio node wired up in `internal/audio/effect.go`.

That same custom node is what the two acknowledged gaps in the engine need:
the compressor that `buildEffect` currently passes through unprocessed, and
the master limiter that `buildGraph` prints a warning about. **Build the node
callback once and all three land together** — which is the strongest argument
for doing the plugin work next rather than later.
