package serve

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTransport records calls so protocol tests need no audio device.
type fakeTransport struct {
	mu      sync.Mutex
	playing bool
	beat    float64
	calls   []string
}

func (f *fakeTransport) Play() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = true
	f.calls = append(f.calls, "play")
	return nil
}

func (f *fakeTransport) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.playing = false
	f.calls = append(f.calls, "stop")
}

func (f *fakeTransport) Seek(beat float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beat = beat
	f.calls = append(f.calls, "seek")
	return nil
}

func (f *fakeTransport) PositionBeats() float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.beat
}

func (f *fakeTransport) Playing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playing
}

// runSession feeds a scripted client session through the server and returns
// every message it wrote, decoded.
func runSession(t *testing.T, tr Transport, input string) []map[string]any {
	t.Helper()

	pr, pw := io.Pipe()
	srv := New(tr, "test")

	done := make(chan error, 1)
	go func() { done <- srv.Run(strings.NewReader(input), pw) }()

	var msgs []map[string]any
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("server wrote invalid JSON line: %q", sc.Text())
				continue
			}
			msgs = append(msgs, m)
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down on stdin EOF")
	}
	pw.Close()
	<-scanDone
	return msgs
}

func TestProtocolSession(t *testing.T) {
	tr := &fakeTransport{}
	msgs := runSession(t, tr,
		`{"id":1,"type":"play"}
{"id":2,"type":"seek","beat":8}
{"id":3,"type":"stop"}
{"id":4,"type":"warp"}
not json at all
`)

	if len(msgs) == 0 || msgs[0]["type"] != "hello" {
		t.Fatalf("first message must be hello, got %v", msgs)
	}
	if v, ok := msgs[0]["protocol_version"].(float64); !ok || int(v) != ProtocolVersion {
		t.Errorf("hello protocol_version = %v, want %d", msgs[0]["protocol_version"], ProtocolVersion)
	}

	// Collect replies by id (events interleave, so filter).
	byID := map[float64]map[string]any{}
	for _, m := range msgs {
		if id, ok := m["id"].(float64); ok {
			byID[id] = m
		}
	}
	for _, id := range []float64{1, 2, 3} {
		if byID[id] == nil || byID[id]["type"] != "ok" {
			t.Errorf("command %v: reply = %v, want ok", id, byID[id])
		}
	}
	if byID[4] == nil || byID[4]["type"] != "error" {
		t.Errorf("unknown command must get an error reply, got %v", byID[4])
	}

	// The bad JSON line must produce an error EVENT (no id) and not kill the
	// session — commands after it were still answered above.
	foundBadJSON := false
	for _, m := range msgs {
		if m["type"] == "error" && m["id"] == nil {
			foundBadJSON = true
		}
	}
	if !foundBadJSON {
		t.Error("malformed input line should produce an id-less error event")
	}

	if got := tr.PositionBeats(); got != 8 {
		t.Errorf("seek beat = %v, want 8", got)
	}
}

func TestPositionEventsOnlyWhilePlaying(t *testing.T) {
	tr := &fakeTransport{}
	srv := New(tr, "test")
	srv.posInterval = 10 * time.Millisecond

	pr, pw := io.Pipe()
	cmdR, cmdW := io.Pipe()
	go func() { _ = srv.Run(cmdR, pw) }()

	sc := bufio.NewScanner(pr)
	readMsg := func() map[string]any {
		if !sc.Scan() {
			t.Fatal("stream ended early")
		}
		var m map[string]any
		_ = json.Unmarshal(sc.Bytes(), &m)
		return m
	}

	if m := readMsg(); m["type"] != "hello" {
		t.Fatalf("want hello, got %v", m)
	}

	// While stopped: no traffic. Give the ticker time to (not) fire.
	time.Sleep(50 * time.Millisecond)

	// Start playback → ack + state + ticker events.
	_, _ = io.WriteString(cmdW, `{"id":1,"type":"play"}`+"\n")
	if m := readMsg(); m["type"] != "ok" {
		t.Fatalf("want ok, got %v", m)
	}
	positions := 0
	deadline := time.After(2 * time.Second)
	for positions < 3 {
		select {
		case <-deadline:
			t.Fatal("expected position events while playing")
		default:
		}
		if m := readMsg(); m["type"] == "position" {
			positions++
			if m["playing"] != true {
				t.Errorf("position event playing = %v, want true", m["playing"])
			}
		}
	}

	cmdW.Close() // EOF → shutdown
}
