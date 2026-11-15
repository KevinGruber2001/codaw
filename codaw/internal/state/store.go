// Package state holds the project as it currently is: one place that owns it, for everyone who plays
// it or shows it.
//
// A project in the store is never changed. A new version (a file was saved) replaces it whole, so a
// snapshot someone is reading stays as it was, with no lock, for as long as they read it. That is
// what lets the mixer read the project on every block while the watcher swaps in a new one.
package state

import (
	"sync"
	"sync/atomic"

	"github.com/KevinGruber2001/codaw/internal/project"
)

// Event says the project changed, and to what. What changed is not described: nothing needs to know,
// because whoever reads the project reads all of it.
type Event struct {
	Project *project.Project
}

// Store holds the current project and tells subscribers when it changes.
type Store struct {
	project atomic.Pointer[project.Project]

	subsMu sync.Mutex
	subs   []chan Event
}

// New makes a store holding p.
func New(p *project.Project) *Store {
	s := &Store{}
	s.project.Store(p)
	return s
}

// Get is the current project. It must not be changed: to change the project, Set a new one (start
// from Clone).
func (s *Store) Get() *project.Project { return s.project.Load() }

// Set replaces the project and tells the subscribers.
func (s *Store) Set(p *project.Project) {
	s.project.Store(p)
	s.emit(Event{Project: p})
}

// Subscribe registers a channel that receives an Event on every Set, and returns a function that
// unsubscribes it. The caller picks the buffer size.
//
// A subscriber that falls behind misses events but never news: an event is only dropped when its
// channel is full, that is, when one it has not read yet is already waiting. So read the latest
// project with Get, not from the event, when it matters that it is the latest.
func (s *Store) Subscribe(ch chan Event) func() {
	s.subsMu.Lock()
	s.subs = append(s.subs, ch)
	s.subsMu.Unlock()
	return func() {
		s.subsMu.Lock()
		defer s.subsMu.Unlock()
		for i, sub := range s.subs {
			if sub == ch {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				return
			}
		}
	}
}

// emit never blocks: Set is called by the watcher, which must not wait for a slow subscriber.
func (s *Store) emit(e Event) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
