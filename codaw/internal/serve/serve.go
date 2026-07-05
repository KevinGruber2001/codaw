// Package serve implements CodaW's runtime control channel: newline-delimited
// JSON (NDJSON) over stdio, in the language-server style. A frontend (the VS
// Code extension) spawns `codaw serve <project.toml>` as a child process and
// talks to it over the child's stdin/stdout.
//
// The contract (see docs/ipc.md for the full design):
//
//   - stdout carries ONLY protocol messages; logs go to stderr.
//   - Project data never travels here — the client edits the TOML files and
//     the watcher hot-reloads them. This channel is for ephemeral session
//     state only: transport commands in, position/state events out.
//   - Commands carry an `id` and are acknowledged; events have no id and the
//     client must tolerate event types it doesn't know (forward compat).
//
// Wire example:
//
//	→ {"id":1,"type":"play"}
//	← {"id":1,"type":"ok"}
//	← {"type":"position","beat":2.51,"playing":true}
//	→ {"id":2,"type":"seek","beat":32}
//	← {"id":2,"type":"ok"}
//	→ {"id":3,"type":"stop"}
//	← {"id":3,"type":"ok"}
package serve

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"
)

// ProtocolVersion is announced in the hello message. Bump it on any breaking
// change to the message shapes; the client refuses versions it doesn't know.
const ProtocolVersion = 1

// Transport is the slice of the engine the server drives. An interface (not
// *engine.Engine directly) so tests can run against a fake with no audio
// device, and so serve never grows sneaky dependencies on engine internals.
type Transport interface {
	Play() error
	Stop()
	Seek(beat float64) error
	PositionBeats() float64
	Playing() bool
}

// command is every client→server message. One struct covers all commands —
// with three fields, per-type structs would be ceremony without safety.
type command struct {
	ID   int64   `json:"id"`
	Type string  `json:"type"`
	Beat float64 `json:"beat"` // used by "seek"
}

// Server pumps commands from in, drives the transport, and emits events on out.
type Server struct {
	tr  Transport
	in  io.Reader
	out chan any // all outbound messages funnel through one writer goroutine

	engineVersion string
	posInterval   time.Duration
}

// New creates a server. engineVersion is stamped into the hello message so a
// client can log/display exactly which binary it is talking to.
func New(tr Transport, engineVersion string) *Server {
	return &Server{
		tr:            tr,
		engineVersion: engineVersion,
		out:           make(chan any, 64),
		posInterval:   100 * time.Millisecond, // 10 Hz — smooth enough for a playhead
	}
}

// Run serves the protocol until in reaches EOF (parent closed our stdin =
// parent is gone) or reading fails. It always returns after the writer has
// flushed, so no acknowledged message is lost on shutdown.
func (s *Server) Run(in io.Reader, out io.Writer) error {
	writerDone := make(chan struct{})
	go s.writer(out, writerDone)

	tickerStop := make(chan struct{})
	go s.positionLoop(tickerStop)

	// hello goes out first, before any command is read — the client uses it
	// to verify it can speak our protocol at all.
	s.send(map[string]any{
		"type":             "hello",
		"protocol_version": ProtocolVersion,
		"engine_version":   s.engineVersion,
	})

	// Reader loop — runs on the calling goroutine.
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var cmd command
		if err := json.Unmarshal(line, &cmd); err != nil {
			s.send(map[string]any{"type": "error", "message": fmt.Sprintf("bad json: %v", err)})
			continue
		}
		s.handle(cmd)
	}

	// stdin closed → shut down: stop the ticker, then close the out channel
	// so the writer drains what's queued and exits.
	close(tickerStop)
	close(s.out)
	<-writerDone
	return sc.Err()
}

// handle executes one command and queues its reply.
func (s *Server) handle(cmd command) {
	switch cmd.Type {
	case "play":
		if err := s.tr.Play(); err != nil {
			s.fail(cmd.ID, err)
			return
		}
		s.ok(cmd.ID)
		s.sendState()

	case "stop":
		s.tr.Stop()
		s.ok(cmd.ID)
		s.sendState()

	case "seek":
		if err := s.tr.Seek(cmd.Beat); err != nil {
			s.fail(cmd.ID, err)
			return
		}
		s.ok(cmd.ID)
		s.sendState()

	case "ping":
		s.ok(cmd.ID)

	default:
		s.send(map[string]any{
			"id": cmd.ID, "type": "error",
			"message": fmt.Sprintf("unknown command %q", cmd.Type),
		})
	}
}

// positionLoop emits position events while playing. Throttled to posInterval
// and silent when stopped — state transitions are announced by sendState, so
// an idle session produces zero traffic.
func (s *Server) positionLoop(stop <-chan struct{}) {
	t := time.NewTicker(s.posInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if s.tr.Playing() {
				s.sendState()
			}
		}
	}
}

func (s *Server) sendState() {
	s.send(map[string]any{
		"type":    "position",
		"beat":    s.tr.PositionBeats(),
		"playing": s.tr.Playing(),
	})
}

func (s *Server) ok(id int64) {
	s.send(map[string]any{"id": id, "type": "ok"})
}

func (s *Server) fail(id int64, err error) {
	s.send(map[string]any{"id": id, "type": "error", "message": err.Error()})
}

// send queues a message for the writer. Non-blocking: if the client stops
// reading and the buffer fills, we drop (position events are disposable and
// a client that can't keep up with acks is already broken). Dropping beats
// blocking — a stuck pipe must never back up into the engine.
func (s *Server) send(msg any) {
	select {
	case s.out <- msg:
	default:
		log.Printf("[serve] out buffer full — dropping %T", msg)
	}
}

// writer is the single goroutine that owns the output stream. JSON encoding
// from multiple goroutines onto one pipe would interleave bytes mid-message;
// funnelling through one channel + one encoder makes each line atomic.
func (s *Server) writer(out io.Writer, done chan<- struct{}) {
	defer close(done)
	enc := json.NewEncoder(out) // Encode appends the \n — that IS the framing
	for msg := range s.out {
		if err := enc.Encode(msg); err != nil {
			log.Printf("[serve] write failed (client gone?): %v", err)
			return
		}
	}
}
