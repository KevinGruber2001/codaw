# Roadmap — what it takes to bring CodaW to life

Written 2026-09. This is a sequencing argument, not a wishlist: the ordering
below is chosen so that each milestone unblocks the next, and so that the
things a stranger would judge the project on land before the things only we
would notice.

Status shorthand: **[gap]** = something the code today gets wrong or fakes,
**[new]** = net-new capability.

## Where the project actually is

Real and working: multi-file TOML projects, a beat-accurate timeline, tracks
/buses/master with gain/pan/mute/solo, hot-reload via the watcher, parameter
automation, seek, offline render, a `serve` control channel (protocol v1), and
a VS Code extension with mixer/arrangement/transport views published on Open
VSX. That's a real DAW skeleton — more than most projects in this space get to.

The honest gaps, all visible in the source:

| Gap | Where |
|---|---|
| `compressor` has no DSP — audio passes through unprocessed, with a log line | `internal/engine/fx.go` |
| `master.limiter = true` prints a warning and does nothing | `internal/engine/graph.go` |
| `reverb` is a feedback delay, not a reverb | `internal/audio/effect.go` |
| `bit_depth` accepts 16/24/32 but render always writes **16-bit** | `internal/wav/wav.go`, `internal/engine/render.go` |
| No meters — `serve` sends `position`/`state` only, so the mixer has no levels | `internal/serve/serve.go` |
| Unknown `fx.type` is skipped at runtime instead of failing validation | `internal/project/validate.go` |
| The extension needs `codaw` already on `$PATH` or a manual `codaw.enginePath` | `vscode-extension/src/engine/session.ts` |
| No note data, therefore no instruments — CodaW arranges samples only | project-wide |
| No audio input, therefore no recording | project-wide |

Four of those (compressor, limiter, a real reverb, plugins) are **the same
piece of missing infrastructure**: a custom miniaudio node with a C process
callback. Every effect today is a stock miniaudio node. That's why the plugin
work is the right next thing rather than a distraction — see `docs/plugins.md`.

## Milestone 1 — Own the audio callback

*Unblocks: every DSP item, and the entire plugin ecosystem.*

1. **[new] Custom `ma_node` with a Go-owned C process callback.** One node
   type in `internal/audio` that hands a block of interleaved float32 to a
   DSP implementation and takes it back. Pre-allocated, no locks, no Go calls
   on the audio thread. This is the single highest-leverage commit available.
2. **[gap] Compressor DSP** — the first real user of that node, and the one
   effect the sample project already references.
3. **[gap] Master limiter** — deletes the warning in `buildGraph` and makes
   `limiter = true` mean something.
4. **[gap] A real reverb** (Freeverb-style comb/allpass bank), replacing the
   echo. Keep the same param names so no project file breaks.
5. **Parameter smoothing** in the node, so automation and knob drags stop
   zippering — needed by everything after this point.

Test discipline: these are all deterministic offline-renderable, so each gets
a golden-render test through the existing `NewOfflineEngine` path. Silence in,
silence out; a sine in, a known RMS out.

## Milestone 2 — Plugins, phase 1

*See `docs/plugins.md` for the full decision. Depends on Milestone 1.*

6. **[new] Plugin manifest + resolver.** `plugins/<id>/plugin.toml`, parameter
   declarations, `plugins` listing over `serve`.
7. **[new] Declarative DSP patch runtime** — primitives + control-rate
   expressions, running on the Milestone 1 node.
8. **[gap] Manifest-driven validation.** Unknown `fx.type` and out-of-range
   params become load-time errors with file and line, matching the standard
   the rest of the loader already sets.
9. **[gap] Manifest-driven UI.** The extension's suffix heuristic in
   `fxParams.ts` becomes the *fallback*; real labels, ranges and units come
   from the manifest — for built-ins and third-party plugins alike.
10. **[new] `codaw plugin add|verify`** + `codaw.lock` with content hashes,
    vendored and committed with the song. **Reproducible renders forever is
    the headline feature** — lead with it.

## Milestone 3 — Make it feel like a DAW

*Independent of 1–2; can run in parallel if there's a second pair of hands.*

11. **[new] Meters over `serve`.** A `meters` event (peak + RMS per track/bus/
    master, ~20 Hz, throttled like `position` already is). The mixer UI exists;
    it just has nothing to show. Cheapest visible-polish win in the repo.
12. **[gap] 24/32-bit render.** `bit_depth` is already in the schema and
    already validated — the writer just ignores it. Fix the writer, add float32
    WAV, and stop silently downgrading people's exports.
13. **[new] Loop/punch region + metronome.** Table stakes for actually
    working on a track rather than demoing one.
14. **[gap] Engine bootstrap in the extension.** First run should offer to
    download the matching release binary rather than failing with "is
    `codaw` a codaw binary?". Right now the extension's happy path requires
    the user to have already solved a packaging problem by hand — that is
    where most first-time users are lost.

## Milestone 4 — Notes, and therefore instruments

*The big one. Everything above is arranging recorded audio; this is where
CodaW becomes a place you write music rather than assemble it.*

15. **[new] A note/pattern format in TOML.** The design question is whether a
    pattern is a first-class file (`patterns/verse.toml`) referenced from a
    track, or inline in the track. Given how the project already splits files,
    first-class patterns with clips referencing them is the consistent answer —
    and it makes a pattern reusable and diffable on its own.
16. **[new] A scheduler that emits note events** to instrument nodes, sharing
    the beat clock the transport already owns.
17. **[new] Instrument plugins** — the `kind = "generator"` half of the plugin
    manifest, which was deliberately deferred until note data existed.
18. **[new] A sampler instrument** as the first generator (map samples to
    pitches), because it reuses the decoding path that already works.

## Milestone 5 — Recording, and the rest

19. **[new] Audio input capture** to a WAV in the project, with the resulting
    clip written into the track TOML. miniaudio already provides duplex.
20. **[new] Freeze / bounce a track** to a rendered WAV — the pragmatic answer
    to CPU load and to plugin churn.
21. **[new] Plugins, phase 2 (Wasm)**, when someone credibly hits the
    declarative ceiling. Not before.

## What "bringing it to life" means outside the code

The engineering above is necessary and not sufficient. A code-first DAW lives
or dies on whether someone can *see the point in 60 seconds*:

- **A demo song repo.** One real track — not `testdata/basic` — in its own
  public repo, with the WAV rendered by CI on every push. "This repo is a
  song; here's the diff where I changed the chorus" is the entire pitch, and
  no amount of README prose substitutes for it.
- **A 60-second screen recording**: edit a number in a TOML file, hear the
  change, `git diff`, done. This is the asset that gets posted and shared.
- **CI that renders.** A GitHub Action that renders a project to a WAV on
  every commit proves reproducibility *and* doubles as our regression suite.
- **`docs/` needs an index and a real "write your first project" page.** Today
  `docs/` holds design notes for contributors; there is nothing for a user
  who wants to start a song.
- **Ship the engine binary with the extension**, or fetch it on first run
  (item 14). The current two-step install is the biggest silent drop-off.
- **Pick the audience and say it out loud.** The natural one is programmers
  who make music and are tired of un-diffable, un-reviewable, rot-prone
  project files — not producers looking to switch from Ableton. Aiming at
  the second audience means competing on features we will never win; aiming
  at the first means competing on properties no incumbent can offer at all.

## Sequencing, condensed

```
M1 audio callback ──┬─► compressor / limiter / reverb        (fixes 3 gaps)
                    └─► M2 plugins phase 1 ──► M5.21 wasm    (the ecosystem)

M3 meters / bit depth / loop / bootstrap                     (parallel, polish)

M4 notes ──► instruments ──► sampler                         (the big swing)

M5 recording / freeze                                        (last)
```

If only one thing gets done next, it is **Milestone 1** — it converts three
standing gaps and the whole plugin plan from blocked to buildable.
