package state

import (
	"fmt"
	"sync"

	"github.com/KevinGruber2001/codaw/internal/project"
)

// ─────────────────────────────────────────────
//  Store
// ─────────────────────────────────────────────

// Store is the central state container for a running CodaW session.
// It holds the current project state and manages concurrent access to it.
//
// ── Concurrency primer ──
//
// Go runs code concurrently using goroutines — lightweight threads managed
// by the Go runtime. In CodaW, several goroutines run simultaneously:
//
//   - Main goroutine      — CLI, user input
//   - File watcher        — detects TOML changes, calls Apply()
//   - Audio engine        — reads state 100+ times/sec to fill audio buffers
//   - Future: UI server   — reads state to render the interface
//
// Without protection, two goroutines accessing the same memory simultaneously
// causes a DATA RACE — undefined behavior ranging from wrong values to crashes.
// Go's race detector (`go run -race`) will catch these.
//
// The Store uses a sync.RWMutex to protect the project field:
//   - RLock / RUnlock  → multiple goroutines can READ simultaneously
//   - Lock / Unlock    → only ONE goroutine can WRITE, blocks all readers
//
// This is the right choice because reads vastly outnumber writes.
// The audio engine reads state constantly; writes happen only when
// a file changes or the user moves a slider.
type Store struct {
	// mu protects the project field.
	// Convention: always declare the mutex directly above the field it protects.
	// This makes the relationship obvious to anyone reading the code.
	//
	// sync.RWMutex is a value type, not a pointer — don't copy it.
	// The Store is always passed as a pointer (*Store) to prevent accidental copies.
	mu      sync.RWMutex
	project *project.Project

	// subs is the list of subscriber channels.
	// When Apply() is called, the resulting Event is sent to every channel here.
	//
	// Each subscriber gets its own channel — they don't share one.
	// This way a slow subscriber (e.g. a laggy UI) can't block the audio engine
	// from receiving events.
	//
	// subsMu protects the subs slice itself (adding/removing subscribers
	// can happen concurrently with Apply calls).
	subsMu sync.RWMutex
	subs   []chan Event
}

// New creates a new Store with the given project as initial state.
// This is the constructor — always use New() rather than creating
// a Store literal, because New() can enforce invariants (non-nil project, etc.)
func New(p *project.Project) *Store {
	return &Store{
		project: p,
		// subs starts as nil (empty slice) — subscribers register later.
	}
}

// ─────────────────────────────────────────────
//  Reading state
// ─────────────────────────────────────────────

// Get returns a read-only snapshot of the current project state.
//
// IMPORTANT: This returns the actual pointer, not a copy.
// Callers MUST NOT modify the returned project — they should only read it.
// If you need to modify, use Apply() with a Mutation.
//
// Why return a pointer instead of a copy?
// Copying a large Project struct on every audio callback would be expensive.
// The contract "don't modify" is enforced by convention here.
// In a future version we could return a deep copy or use an immutable
// data structure, but for now the pointer approach is standard Go practice.
//
// Usage pattern in the audio engine:
//
//	p := store.Get()
//	gain := 0.0
//	for _, t := range p.Tracks {
//	    if t.ID == "kick" { gain = t.Gain }
//	}
//	// Never: p.Tracks[0].Gain = 5.0  ← WRONG, use Apply() instead
func (s *Store) Get() *project.Project {
	// RLock allows multiple simultaneous readers.
	// Multiple goroutines can call Get() concurrently — this is the whole
	// point of RWMutex. If we used a regular Lock here, every Get() call
	// would block all other Get() calls, killing audio performance.
	s.mu.RLock()
	defer s.mu.RUnlock()
	// defer runs when the function returns, even if it panics.
	// defer + Unlock is idiomatic Go — it's impossible to forget to unlock.
	return s.project
}

// ─────────────────────────────────────────────
//  Writing state
// ─────────────────────────────────────────────

// Apply runs a Mutation against the current project state, then notifies
// all subscribers about what changed.
//
// This is the ONLY way to modify state in CodaW.
// Never modify the project directly — always go through Apply().
//
// The flow:
//  1. Acquire exclusive write lock (blocks all readers until we're done)
//  2. Run the mutation function (modifies project in place)
//  3. Release write lock
//  4. Emit the resulting event to all subscribers
//
// Note: we release the lock BEFORE emitting events.
// Why? Because emitting to a channel could block (if a subscriber's channel
// is full), and we don't want to hold the write lock while blocked.
// This means subscribers receive events slightly after the state changes,
// but that's fine — they call Get() to read the new state anyway.
func (s *Store) Apply(m Mutation) {
	// Step 1 + 2 + 3 — lock, mutate, unlock
	var event Event
	func() {
		// We wrap this in an anonymous function so we can use defer
		// for the unlock while still capturing the event return value.
		s.mu.Lock()
		defer s.mu.Unlock()
		event = m(s.project)
	}()

	// Step 4 — emit event (lock is already released)
	// Only emit if the mutation returned a meaningful event.
	// A zero-value Event (empty Type) means "nothing matched / nothing changed".
	if event.Type != "" {
		s.emit(event)
	}
}

// Reload replaces the entire project with a freshly loaded one.
// Used by the file watcher when a structural change is detected
// (e.g. a new track added to project.toml's layout).
//
// This is heavier than Apply() — it swaps the whole project,
// so the audio engine needs to fully rebuild its graph.
func (s *Store) Reload(p *project.Project) {
	s.mu.Lock()
	s.project = p
	s.mu.Unlock()

	s.emit(Event{Type: EventProjectReloaded})
}

// ─────────────────────────────────────────────
//  Subscriptions
// ─────────────────────────────────────────────

// Subscribe registers a new subscriber channel.
// The caller provides the channel — this gives them control over buffer size.
//
// Returns a function that unsubscribes when called.
// This "return a cleanup function" pattern is common in Go:
//
//	ch := make(chan Event, 32)
//	unsub := store.Subscribe(ch)
//	defer unsub()  // clean up when done
//
// Why provide the channel rather than creating it internally?
// The caller knows best what buffer size they need.
// A UI subscriber might want a large buffer (64+) to smooth out bursts.
// The audio engine might want a small buffer (8) and drop old events.
//
// Why buffered channels?
// If we used unbuffered channels (make(chan Event)), sending to a subscriber
// would BLOCK until they read from it. If the audio engine is busy for
// 10ms, Apply() would stall — terrible for real-time audio.
// Buffered channels let Apply() send and move on immediately.
// If the buffer fills up, we drop the event (see emit() below) — a dropped
// event is far better than a blocked audio thread.
func (s *Store) Subscribe(ch chan Event) func() {
	s.subsMu.Lock()
	s.subs = append(s.subs, ch)
	s.subsMu.Unlock()

	// Return an unsubscribe function — a closure that captures ch.
	return func() {
		s.subsMu.Lock()
		defer s.subsMu.Unlock()

		// Find and remove this channel from the subs slice.
		// We iterate to find the index, then remove it with slice tricks.
		for i, sub := range s.subs {
			if sub == ch {
				// Remove element at index i without preserving order.
				// s.subs[i] = s.subs[len-1]  ← move last to position i
				// s.subs = s.subs[:len-1]     ← shrink slice by 1
				// This is O(1) vs O(n) for an order-preserving removal.
				// Order doesn't matter for subscribers.
				s.subs[i] = s.subs[len(s.subs)-1]
				s.subs = s.subs[:len(s.subs)-1]
				return
			}
		}
	}
}

// emit sends an event to all current subscribers.
// It is called after the write lock is released.
func (s *Store) emit(e Event) {
	s.subsMu.RLock()
	defer s.subsMu.RUnlock()

	for _, ch := range s.subs {
		// Non-blocking send using the select + default pattern.
		//
		// A regular send (ch <- e) would block if the channel buffer is full.
		// We never want emit() to block — it's called from Apply() which
		// may be called from the file watcher or UI handler.
		//
		// select checks multiple channel operations simultaneously.
		// The default case runs immediately if no other case is ready.
		// This means: "try to send; if the buffer is full, drop the event."
		//
		// Dropping an event is acceptable here because:
		// - The subscriber's buffer being full means they're already behind
		// - They'll catch up when they process buffered events
		// - The audio engine re-reads state on every buffer fill anyway
		select {
		case ch <- e:
			// sent successfully
		default:
			// channel full — drop event
			// In production you might log this or track a metric
			fmt.Printf("[state] warning: subscriber channel full, dropping event %s\n", e.Type)
		}
	}
}

// ─────────────────────────────────────────────
//  Convenience methods
// ─────────────────────────────────────────────

// BPM returns the current project BPM.
// A convenience wrapper around Get() for frequently accessed fields.
// The audio engine calls this constantly — having a named method is cleaner
// than store.Get().Transport.BPM everywhere.
func (s *Store) BPM() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.project.Transport.BPM
}

// SampleRate returns the project sample rate.
func (s *Store) SampleRate() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.project.Transport.SampleRate
}
