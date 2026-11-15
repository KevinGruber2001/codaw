<p align="center">
  <img src="assets/logo.png" width="128" alt="CodaW logo" />
</p>

<h1 align="center">CodaW</h1>

<p align="center">A code-first digital audio workstation — your whole project lives in plain-text TOML.</p>

<p align="center">
  <a href="https://open-vsx.org/extension/kevingruber/codaw"><img src="https://img.shields.io/open-vsx/v/kevingruber/codaw?label=open%20vsx&color=FF6A5A" alt="Open VSX version" /></a>
  <img src="https://img.shields.io/badge/license-MIT-FF6A5A" alt="MIT license" />
</p>

---

CodaW is a DAW where the session is code you can version, diff, and hand-edit. No hunting through mixer pages — you edit the file for the thing you want to change, save, and the running audio engine picks it up live.

## Repo layout

| Path | What |
|------|------|
| [`codaw/`](codaw/) | The audio engine — a Go CLI with its own mixer and effects (malgo, via cgo, for the audio device). Loads/validates a project, plays it, hot-reloads on edit, and renders to WAV. |
| [`vscode-extension/`](vscode-extension/) | The editor UI, published on Open VSX as `kevingruber.codaw`. |
| [`editor/`](editor/) | The standalone CodaW application — a desktop and browser editor built on [Eclipse Theia](https://theia-ide.org). Early: branding and shell so far. |

## Install

Grab a prebuilt binary for your OS from the
[latest release](https://github.com/KevinGruber2001/codaw/releases) — no
toolchain needed.

Building from source requires Go plus a C compiler (the engine reaches the audio
device through [malgo](https://github.com/gen2brain/malgo), a cgo binding). Note that `go install .../codaw@latest`
is deliberately unsupported: the Go module lives in the `codaw/` subdirectory of
this monorepo, and cgo would require every user to carry a C toolchain anyway —
prebuilt release binaries are the supported channel.

## Quick start (engine)

```bash
cd codaw
go run ./cmd/codaw validate testdata/basic/project.toml   # check a project
go run ./cmd/codaw play     testdata/basic/project.toml   # play it (Ctrl-C to stop)
go run ./cmd/codaw watch    testdata/basic/project.toml   # play + hot-reload as you edit
go run ./cmd/codaw render   testdata/basic/project.toml mix.wav   # bounce to a WAV
```

While `watch` is running, edit any `.toml` under the project (a track's gain, an FX
param, an automation point) and hear the change instantly.

## What works today

- Multi-file TOML projects: tracks, buses, master, clips, FX, automation
- Playback with a beat-accurate timeline, per-track gain/pan/mute/solo, bus routing
- Live hot-reload (edit a file → audible change, no restart)
- Effects, all in Go: 3-band EQ, compressor, reverb (Freeverb), master limiter
- Non-destructive clip trim/split (source in-points)
- Parameter automation over time (gain fades, filter sweeps)
- Offline render to 16-bit WAV, through the same mixer as playback

## License

[MIT](LICENSE)
