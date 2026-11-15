// Package watcher monitors project TOML files for changes and applies
// them to the state store reactively.
//
// When a file changes on disk, the watcher reads it, checks that the project is still valid with it,
// and puts the new project in the store. It does not work out what changed: the engine reads the
// whole project anyway, and plays whatever is new from its next block.
package watcher

import (
	"fmt"
	"log"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/schema"
	"github.com/KevinGruber2001/codaw/internal/state"
	"github.com/fsnotify/fsnotify"
)

// ─────────────────────────────────────────────
//  Watcher
// ─────────────────────────────────────────────

// Watcher monitors TOML files and reacts to changes.
type Watcher struct {
	// store is the state store we apply mutations to.
	store *state.Store

	// fsw is the underlying filesystem watcher from fsnotify.
	// fsnotify uses OS-level file system events (inotify on Linux,
	// kqueue on macOS, ReadDirectoryChangesW on Windows) — much more
	// efficient than polling files every N milliseconds.
	fsw *fsnotify.Watcher

	// done is a channel we close to signal the watch goroutine to stop.
	// Closing a channel (rather than sending a value) is the idiomatic
	// Go way to broadcast "stop" to multiple goroutines — all receivers
	// see a closed channel immediately.
	done chan struct{}

	// rejected are files whose last change would have made the project invalid. They are tried again
	// with every later change: renaming a bus takes two saves, and neither is valid without the other.
	rejected map[string]bool
}

// New creates a Watcher that monitors all project files and applies
// changes to the given store.
//
// Returns an error if the OS-level watcher can't be created
// (rare, usually a system resource limit issue).
func New(store *state.Store) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("could not create file watcher: %w", err)
	}

	return &Watcher{
		store: store,
		fsw:   fsw,
		done:  make(chan struct{}),

		rejected: make(map[string]bool),
	}, nil
}

// Start begins watching all files referenced by the current project.
// It registers each file with fsnotify and starts the event loop in
// a background goroutine.
//
// The goroutine pattern: Start() returns immediately. The actual watching
// happens concurrently in a new goroutine. This is standard Go — you never
// want to block the caller with a long-running loop.
//
// Stop() closes the done channel, causing the goroutine to exit cleanly.
func (w *Watcher) Start() error {
	p := w.store.Get()

	// Register the project root directory.
	// We watch the directory rather than individual files because:
	// 1. Some editors (vim, emacs) save by writing a temp file then renaming —
	//    this doesn't trigger a file modify event, but does trigger a create event
	//    in the directory. Watching the directory catches both.
	// 2. New files added to tracks/ or buses/ are automatically noticed.
	if err := w.fsw.Add(p.RootDir); err != nil {
		return fmt.Errorf("could not watch %s: %w", p.RootDir, err)
	}

	// Also watch subdirectories (tracks/, buses/)
	for _, dir := range []string{"tracks", "buses"} {
		path := filepath.Join(p.RootDir, dir)
		// It's okay if these directories don't exist yet
		_ = w.fsw.Add(path)
	}

	// Start the event loop in a goroutine.
	// The `go` keyword launches a new goroutine — a concurrent function.
	// This goroutine runs the watch loop independently of the caller.
	go w.loop()

	log.Printf("[watcher] watching %s", p.RootDir)
	return nil
}

// Stop shuts down the watcher and its goroutine cleanly.
func (w *Watcher) Stop() {
	// Closing the done channel signals the goroutine to exit.
	// This is safe to call multiple times — closing a closed channel panics,
	// but we only close it once here.
	close(w.done)
	w.fsw.Close()
}

// ─────────────────────────────────────────────
//  Event loop
// ─────────────────────────────────────────────

const debounce = 50 * time.Millisecond

// loop runs until Stop. It waits for the saves to settle (an editor writes a file in several steps),
// then handles every file that changed. The list of changed files and the timer belong to this
// goroutine alone.
func (w *Watcher) loop() {
	pending := make(map[string]bool)
	timer := time.NewTimer(debounce)
	timer.Stop()
	defer timer.Stop()

	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) && filepath.Ext(event.Name) == ".toml" {
				pending[event.Name] = true
				timer.Reset(debounce)
			}

		case <-timer.C:
			files := make([]string, 0, len(pending))
			for file := range pending {
				files = append(files, file)
			}
			clear(pending)
			w.handleChanges(files)

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("[watcher] error: %v", err)

		case <-w.done:
			return
		}
	}
}

// ─────────────────────────────────────────────
//  Change handling
// ─────────────────────────────────────────────

// member is one track, bus or master file as it is on disk now.
type member struct {
	file   string
	track  *project.Track
	bus    *project.Bus
	master *project.Master
}

func (w *Watcher) handleChange(file string) {
	w.handleChanges([]string{file})
}

// handleChanges applies the files that changed, together with those rejected earlier. All at once if
// that leaves the project valid; otherwise each on its own, and what is still invalid waits for the
// change that completes it.
func (w *Watcher) handleChanges(files []string) {
	p := w.store.Get()
	changed := make(map[string]bool, len(files))
	for _, file := range files {
		if file == filepath.Join(p.RootDir, "project.toml") {
			clear(w.rejected)
			w.handleProjectChange(file)
			return
		}
		changed[file] = true
	}
	for file := range w.rejected {
		files = append(files, file)
	}
	sort.Strings(files)

	var members []member
	for i, file := range files {
		if i > 0 && file == files[i-1] {
			continue
		}
		if m, ok := readMember(p, file); ok {
			members = append(members, m)
		} else {
			delete(w.rejected, file)
		}
	}
	if len(members) == 0 {
		return
	}

	if next := merge(p, members); project.Validate(next) == nil {
		for _, m := range members {
			delete(w.rejected, m.file)
		}
		w.set(p, next, members)
		return
	}
	for _, m := range members {
		p := w.store.Get()
		next := merge(p, []member{m})
		if err := project.Validate(next); err != nil {
			if changed[m.file] {
				logRejected(m.file, err)
			}
			w.rejected[m.file] = true
			continue
		}
		delete(w.rejected, m.file)
		w.set(p, next, []member{m})
	}
}

// set puts the new project in the store, unless the files were saved without a change.
func (w *Watcher) set(old, next *project.Project, members []member) {
	if reflect.DeepEqual(normalised(old), normalised(next)) {
		return
	}
	w.store.Set(next)
	for _, m := range members {
		log.Printf("[watcher] applied %s", filepath.Base(m.file))
	}
}

// readMember reads a file of the project from disk. False for a file that is not one of the
// project's, or that does not parse: the engine keeps what it has.
func readMember(p *project.Project, file string) (member, bool) {
	m := member{file: file}
	var into any
	switch {
	case p.Master != nil && file == p.Master.SourceFile:
		m.master = &project.Master{SourceFile: file}
		into = m.master
	case findBus(p, file) != nil:
		m.bus = &project.Bus{SourceFile: file}
		into = m.bus
	case findTrack(p, file) != nil:
		m.track = &project.Track{SourceFile: file}
		into = m.track
	default:
		return m, false
	}
	if _, err := toml.DecodeFile(file, into); err != nil {
		log.Printf("[watcher] failed to parse %s: %v", file, err)
		return m, false
	}
	return m, true
}

// merge is the project as it is with the given files read again. A clone: the store's project is
// never changed.
func merge(p *project.Project, members []member) *project.Project {
	next := p.Clone()
	for _, m := range members {
		switch {
		case m.master != nil:
			master := *m.master
			next.Master = &master
		case m.bus != nil:
			for i, b := range next.Buses {
				if b.SourceFile == m.file {
					bus := *m.bus
					next.Buses[i] = &bus
				}
			}
		case m.track != nil:
			for i, t := range next.Tracks {
				if t.SourceFile == m.file {
					track := *m.track
					next.Tracks[i] = &track
				}
			}
		}
	}
	return next
}

// normalised is a project with empty lists as nil, so one read from a file and its clone in the
// store compare equal when nothing changed.
func normalised(p *project.Project) *project.Project {
	c := p.Clone()
	for _, t := range c.Tracks {
		if len(t.FX) == 0 {
			t.FX = nil
		}
		if len(t.Clips) == 0 {
			t.Clips = nil
		}
		if len(t.Automation) == 0 {
			t.Automation = nil
		}
		for i := range t.FX {
			t.FX[i].Params = numbersAsFloats(t.FX[i].Params)
		}
	}
	for _, b := range c.Buses {
		if len(b.FX) == 0 {
			b.FX = nil
		}
		for i := range b.FX {
			b.FX[i].Params = numbersAsFloats(b.FX[i].Params)
		}
	}
	if len(c.Master.FX) == 0 {
		c.Master.FX = nil
	}
	for i := range c.Master.FX {
		c.Master.FX[i].Params = numbersAsFloats(c.Master.FX[i].Params)
	}
	return c
}

// numbersAsFloats makes `wet = 1` and `wet = 1.0` the same value.
func numbersAsFloats(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for key, value := range params {
		if n, ok := schema.AsNumber(value); ok {
			value = n
		}
		out[key] = value
	}
	return out
}

func findTrack(p *project.Project, file string) *project.Track {
	for _, t := range p.Tracks {
		if t.SourceFile == file {
			return t
		}
	}
	return nil
}

func findBus(p *project.Project, file string) *project.Bus {
	for _, b := range p.Buses {
		if b.SourceFile == file {
			return b
		}
	}
	return nil
}

// handleProjectChange does a full reload when project.toml itself changes.
// This handles layout changes (new tracks added, buses reordered, etc.)
func (w *Watcher) handleProjectChange(file string) {
	log.Printf("[watcher] project.toml changed — reloading")

	p, err := project.Load(file)
	if err != nil {
		log.Printf("[watcher] reload failed: %v", err)
		return
	}
	if err := project.Validate(p); err != nil {
		log.Printf("[watcher] reload aborted — validation failed:\n%v", err)
		return
	}

	w.store.Set(p)
	log.Printf("[watcher] project reloaded successfully")
}

func logRejected(file string, err error) {
	log.Printf("[watcher] %s not applied, it would make the project invalid (still playing the last valid version):\n%v",
		filepath.Base(file), err)
}
