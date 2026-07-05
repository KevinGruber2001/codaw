# Runtime control channel — design notes

Status: **v1 implemented** — `codaw serve` (engine: `internal/serve`, protocol
version 1) and the extension-side client (`vscode-extension/src/engineClient.ts`).
Decision: **stdio JSON-lines first, Unix socket later if multi-client becomes real.**

## The two planes of data

CodaW already has a complete answer for *project* data: TOML files are the
source of truth, every consumer (engine, extension, human with a text editor)
reads and writes those files, and the watcher hot-reloads the engine. That
channel is durable, diffable, git-versioned — and it must stay the ONLY way
project data moves. Adding a second write path for project data (e.g. "set
gain over the socket") would create two writers with different views and
guarantee eventual divergence.

What's missing is the *ephemeral* plane: data about the current **session**
that has no business being in a file because it changes 20 times a second or
describes a running process, not a composition:

| Direction | Examples |
|---|---|
| commands → engine | play, stop, seek to beat N, toggle metronome |
| events ← engine | playhead position, playing/stopped, meter levels, "graph rebuilt", errors |

The rule that keeps the architecture clean: **files carry what you'd commit;
the channel carries what you'd never commit.**

## Alternatives considered

### A. stdio JSON-lines (extension spawns `codaw serve` as a child process)

The Language Server Protocol model: one process per consumer, newline-delimited
JSON on stdin/stdout.

- ✅ Zero networking: no ports, no firewalls, no discovery, no auth story
  (the pipe *is* the auth — only the parent holds it)
- ✅ Lifecycle for free: child dies with the parent, no orphaned engines
- ✅ Trivially cross-platform (stdio behaves identically on Windows)
- ✅ ~50 lines of Go (`bufio.Scanner` on stdin, `json.Encoder` on stdout)
- ✅ The entire VS Code ecosystem runs on this pattern (every language server)
- ❌ Exactly one client per engine process
- ❌ Can't attach a debugging client to a running session
- ❌ stdout is sacred — a stray `fmt.Println` corrupts the protocol
  (logs must go to stderr)

### B. Unix domain socket (+ named pipe on Windows)

The original plan from the project summary ("Swift UI via Unix socket").

- ✅ Multi-client: extension + CLI + future Swift app can share one engine
- ✅ Still no network exposure (filesystem permissions gate access)
- ✅ Same JSON-lines framing works verbatim
- ❌ Lifecycle is now a real problem: who starts the engine? who kills it?
  stale socket files after a crash? two engines racing for one path?
- ❌ Windows needs a separate named-pipe code path (Go: different packages)
- ❌ Discovery convention needed (socket path in the project dir? `$TMPDIR`?)

### C. WebSocket on localhost

- ✅ Multi-client, and a *browser* could connect (web UI someday)
- ✅ One code path on every OS
- ❌ Opens a TCP port: port collisions, "allow network?" firewall prompts,
  and any local process can connect — needs a token handshake
- ❌ Heavier dependency and lifecycle story than either A or B
- ❌ Solves a problem (remote/browser clients) CodaW doesn't have yet

### D. HTTP REST + Server-Sent Events

- ✅ Ubiquitous tooling, curl-able for debugging
- ❌ Request/response is the wrong shape for a 20 Hz event stream
- ❌ Same port/firewall drawbacks as C with more ceremony

### E. gRPC

- ✅ Typed schema, codegen, streaming built in
- ❌ Massive toolchain cost (protoc, generated TS + Go) for ~6 message types
- ❌ Optimises for problems (many teams, many services, API evolution at
  scale) that a solo two-process app does not have

### F. State file + fsnotify (write playhead to a file, watch it)

Listed only because it's tempting given the watcher already exists:
- ❌ 20 writes/sec of throwaway data through the filesystem, debounce fights,
  SSD churn, and the watcher would have to *ignore* these files anyway.
  The file plane is for durable data; abusing it for ephemera breaks the
  one rule that keeps the two planes clean. Rejected.

## Decision

**Phase 1 — now: option A.** `codaw serve <project.toml>` speaking JSON-lines
over stdio. The extension owns the process (spawn on open, kill on close),
which eliminates the entire lifecycle/discovery problem class on day one.
This is the boring, battle-tested choice: it is literally how every language
server ships.

**Phase 2 — only when a second simultaneous client actually exists** (Swift
app, standalone mixer CLI): lift the same protocol onto a Unix socket /
named pipe. Because framing and messages don't change, `serve` grows a
`--listen <path>` flag and the stdio path stays for the extension. The
protocol is transport-agnostic by construction, so this is additive.

Skip C/D/E unless a browser client or remote control becomes a real
requirement.

## Protocol sketch (v1)

Newline-delimited JSON. Every message has a `type`. Client→engine messages
carry an `id` echoed in the reply so requests can be correlated.

```
→ {"id":1,"type":"play"}
← {"id":1,"type":"ok"}
→ {"id":2,"type":"seek","beat":32}
← {"id":2,"type":"ok"}
← {"type":"position","beat":32.5,"playing":true}      (event, ~10–20 Hz)
← {"type":"state","playing":false}                    (event, on change)
← {"type":"error","message":"..."}                    (event)
→ {"id":3,"type":"stop"}
← {"id":3,"type":"ok"}
```

Ground rules:

1. **`protocol_version` in the hello message.** First message from the engine:
   `{"type":"hello","protocol_version":1,"engine_version":"0.1.3"}`. The
   extension refuses to talk to a version it doesn't know. Costs one field,
   saves a debugging weekend later.
2. **Project edits do NOT travel over this channel.** The UI writes TOML; the
   watcher picks it up. One writer per plane. (If knob-drag latency ever
   demands a fast path, add an explicitly-labelled *preview* message that
   never persists — the TOML save on mouse-up remains the commit.)
3. **Position events are throttled** (~10–20 Hz) and carry beats, not frames —
   the UI thinks in musical time.
4. **stdout = protocol, stderr = logs.** The engine's `log` package already
   writes to stderr by default; keep it that way.
5. **Events are fire-and-forget; commands are acknowledged.** The client must
   tolerate events it doesn't understand (forward compatibility).

## Engine-side shape

`internal/serve` package: reads commands, drives the engine, publishes events.
It layers on exactly what exists — `engine.Play/Stop` and the store's
subscribe mechanism — plus two things the engine must grow anyway:

- `engine.Seek(beat float64)` — reschedule clips relative to a new playhead
  (falls out of the transport work; today playback always starts at beat 0)
- a position getter (`engine.PositionBeats()` already exists in spirit as
  `positionBeatsLocked`)

Neither the state store nor the watcher changes at all.
