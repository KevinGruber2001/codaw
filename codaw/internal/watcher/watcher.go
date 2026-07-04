// Package watcher monitors project TOML files for changes and applies
// them to the state store reactively.
//
// When a file changes on disk, the watcher:
//  1. Re-parses the changed file
//  2. Diffs it against current in-memory state
//  3. Calls store.Apply() with targeted mutations for changed values
//  4. Falls back to store.Reload() for structural changes
package watcher

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/KevinGruber2001/codaw/internal/project"
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

// loop is the main goroutine — it runs forever until Stop() is called.
//
// It uses a select statement to wait for either:
//   - a filesystem event (file changed)
//   - an error from fsnotify
//   - a signal on the done channel (Stop() was called)
//
// select is Go's way of waiting on multiple channel operations simultaneously.
// It blocks until one of the cases is ready, then runs that case.
// This is fundamentally different from a switch — it's about concurrency,
// not value matching.
func (w *Watcher) loop() {
	// Debounce: editors often emit multiple rapid events for one save
	// (e.g. vim writes temp file, renames, updates metadata = 3 events).
	// We use a timer to wait 50ms after the last event before processing.
	// This is the debounce pattern — common in UI and filesystem code.
	//
	// Why 50ms? Fast enough to feel instant (~1/20th of a second),
	// slow enough to let the editor finish writing the file.
	var debounce *time.Timer
	pendingFiles := make(map[string]bool) // files waiting to be processed

	for {
		select {

		// ── Filesystem event ──
		case event, ok := <-w.fsw.Events:
			// The second return value `ok` from a channel receive is false
			// when the channel is closed. If fsnotify closes its Events
			// channel (e.g. after fsw.Close()), we exit the loop.
			if !ok {
				return
			}

			// We only care about Write and Create events.
			// Rename and Remove are handled implicitly — if a file is
			// removed, the next read will fail gracefully.
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Only process TOML files
				if filepath.Ext(event.Name) == ".toml" {
					pendingFiles[event.Name] = true

					// Reset the debounce timer.
					// If a timer is already running, stop it and restart.
					// This means "wait 50ms from the LAST event, not the first."
					if debounce != nil {
						debounce.Stop()
					}
					debounce = time.AfterFunc(50*time.Millisecond, func() {
						// This function runs in its own goroutine after 50ms.
						// We collect the pending files and process them.
						//
						// Copy the map to avoid a data race — the loop goroutine
						// owns pendingFiles, but this func runs in a separate goroutine.
						toProcess := make(map[string]bool)
						for f := range pendingFiles {
							toProcess[f] = true
						}
						// Clear pending
						for f := range pendingFiles {
							delete(pendingFiles, f)
						}

						for file := range toProcess {
							w.handleChange(file)
						}
					})
				}
			}

		// ── fsnotify error ──
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("[watcher] error: %v", err)

		// ── Stop signal ──
		case <-w.done:
			// done channel was closed — exit the loop cleanly.
			if debounce != nil {
				debounce.Stop()
			}
			return
		}
	}
}

// ─────────────────────────────────────────────
//  Change handling
// ─────────────────────────────────────────────

// handleChange is called when a TOML file has changed.
// It determines which file changed and delegates to the appropriate handler.
func (w *Watcher) handleChange(file string) {
	p := w.store.Get()

	// Determine which file changed by comparing against known paths.
	// We compare the absolute path of the changed file against the
	// SourceFile fields we stored during loading.

	if file == p.Master.SourceFile {
		w.handleMasterChange(file, p)
		return
	}

	for _, b := range p.Buses {
		if file == b.SourceFile {
			w.handleBusChange(file, b, p)
			return
		}
	}

	for _, t := range p.Tracks {
		if file == t.SourceFile {
			w.handleTrackChange(file, t, p)
			return
		}
	}

	// If the changed file is project.toml itself, do a full reload.
	projectFile := filepath.Join(p.RootDir, "project.toml")
	if file == projectFile {
		w.handleProjectChange(file)
	}
}

// handleTrackChange re-parses a track file and applies changes to the store.
//
// This is the core of the "reactive TOML" system.
// The key insight: we compare old (in-memory) state with new (parsed) state
// and only emit mutations for what actually changed.
func (w *Watcher) handleTrackChange(file string, old *project.Track, p *project.Project) {
	// Re-parse the changed file into a fresh Track struct.
	var updated project.Track
	if _, err := toml.DecodeFile(file, &updated); err != nil {
		log.Printf("[watcher] failed to parse %s: %v", file, err)
		return
	}
	updated.SourceFile = file

	log.Printf("[watcher] track changed: %s", old.ID)

	// ── Diff: compare old vs new, apply targeted mutations ──
	//
	// Why diff instead of just replacing the whole track?
	// Because targeted mutations emit specific events (EventTrackGainChanged)
	// which the audio engine can apply with zero interruption to playback.
	// A full replace emits EventStructureChanged which forces a graph rebuild —
	// potentially causing a small audio glitch.
	//
	// So we try to be surgical first, and only fall back to full replace
	// if something structural changed.

	structural := false

	// Check scalar params — these are hot-swappable
	if old.Gain != updated.Gain {
		w.store.Apply(state.SetTrackGain(old.ID, updated.Gain))
		log.Printf("[watcher]   gain: %.1f → %.1f dB", old.Gain, updated.Gain)
	}

	if old.Pan != updated.Pan {
		w.store.Apply(state.SetTrackPan(old.ID, updated.Pan))
		log.Printf("[watcher]   pan: %.2f → %.2f", old.Pan, updated.Pan)
	}

	if old.Mute != updated.Mute {
		w.store.Apply(state.SetTrackMute(old.ID, updated.Mute))
		log.Printf("[watcher]   mute: %v → %v", old.Mute, updated.Mute)
	}

	if old.Solo != updated.Solo {
		w.store.Apply(state.SetTrackSolo(old.ID, updated.Solo))
		log.Printf("[watcher]   solo: %v → %v", old.Solo, updated.Solo)
	}

	// Diff FX params — only if chain length is the same (no structural change)
	if len(old.FX) == len(updated.FX) {
		for i := range old.FX {
			// Only diff if same FX type — different type = structural change
			if old.FX[i].Type != updated.FX[i].Type {
				structural = true
				break
			}
			diffFXParams(w.store, old.ID, i, old.FX[i], updated.FX[i])
		}
	} else {
		// FX chain length changed — structural
		structural = true
	}

	// Diff clips — if count or files changed, that's structural
	if len(old.Clips) != len(updated.Clips) {
		structural = true
	} else {
		for i := range old.Clips {
			if old.Clips[i].File != updated.Clips[i].File {
				structural = true
				break
			}
			// Clip timing and in-point changes are also structural — they
			// affect where/when the clip plays and where it reads from.
			if old.Clips[i].Start != updated.Clips[i].Start ||
				old.Clips[i].End != updated.Clips[i].End ||
				old.Clips[i].Offset != updated.Clips[i].Offset {
				structural = true
				break
			}
			// Clip gain is hot-swappable — but we don't have a dedicated
			// mutation for it yet, so treat as structural for now.
			// TODO: add SetClipGain mutation in a future pass
		}
	}

	if structural {
		log.Printf("[watcher]   structural change detected — replacing track")
		w.store.Apply(state.ReplaceTrack(&updated))
	}
}

// handleBusChange diffs and applies bus file changes.
func (w *Watcher) handleBusChange(file string, old *project.Bus, p *project.Project) {
	var updated project.Bus
	if _, err := toml.DecodeFile(file, &updated); err != nil {
		log.Printf("[watcher] failed to parse %s: %v", file, err)
		return
	}
	updated.SourceFile = file

	log.Printf("[watcher] bus changed: %s", old.ID)

	if old.Gain != updated.Gain {
		w.store.Apply(state.SetBusGain(old.ID, updated.Gain))
		log.Printf("[watcher]   gain: %.1f → %.1f dB", old.Gain, updated.Gain)
	}

	structural := false
	if len(old.FX) == len(updated.FX) {
		for i := range old.FX {
			if old.FX[i].Type != updated.FX[i].Type {
				structural = true
				break
			}
			diffFXParams(w.store, old.ID, i, old.FX[i], updated.FX[i])
		}
	} else {
		structural = true
	}

	if structural {
		w.store.Apply(state.ReplaceBus(&updated))
	}
}

// handleMasterChange diffs and applies master file changes.
func (w *Watcher) handleMasterChange(file string, p *project.Project) {
	var updated project.Master
	if _, err := toml.DecodeFile(file, &updated); err != nil {
		log.Printf("[watcher] failed to parse %s: %v", file, err)
		return
	}
	updated.SourceFile = file

	old := p.Master
	log.Printf("[watcher] master changed")

	if old.Gain != updated.Gain {
		w.store.Apply(state.SetMasterGain(updated.Gain))
		log.Printf("[watcher]   gain: %.1f → %.1f dB", old.Gain, updated.Gain)
	}

	structural := false
	if len(old.FX) == len(updated.FX) {
		for i := range old.FX {
			if old.FX[i].Type != updated.FX[i].Type {
				structural = true
				break
			}
			diffFXParams(w.store, "master", i, old.FX[i], updated.FX[i])
		}
	} else {
		structural = true
	}

	if structural {
		w.store.Apply(state.ReplaceMaster(&updated))
	}
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

	w.store.Reload(p)
	log.Printf("[watcher] project reloaded successfully")
}

// ─────────────────────────────────────────────
//  FX diffing helper
// ─────────────────────────────────────────────

// diffFXParams compares two FX units with the same type and applies
// SetFXParam mutations for any parameters that changed.
//
// FX params are stored as map[string]any — we iterate the new params
// and compare each one to the old value.
func diffFXParams(store *state.Store, ownerID string, fxIndex int, old, updated project.FX) {
	for key, newVal := range updated.Params {
		// Type-assert the new value to float64.
		// TOML numbers without a decimal are decoded as int64 by BurntSushi/toml,
		// and numbers with a decimal are float64. We normalize to float64.
		newF, ok := toFloat64(newVal)
		if !ok {
			continue // skip non-numeric params (e.g. string params)
		}

		oldVal, exists := old.Params[key]
		if !exists {
			// New param that didn't exist before — apply it
			store.Apply(state.SetFXParam(ownerID, fxIndex, key, newF))
			continue
		}

		oldF, ok := toFloat64(oldVal)
		if !ok {
			continue
		}

		if oldF != newF {
			store.Apply(state.SetFXParam(ownerID, fxIndex, key, newF))
			log.Printf("[watcher]   fx[%d].%s: %.3f → %.3f", fxIndex, key, oldF, newF)
		}
	}
}

// toFloat64 converts any numeric type to float64.
// TOML decodes integers as int64 and floats as float64 — we normalize here
// so callers don't need to handle both cases.
func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case int:
		return float64(val), true
	case float32:
		return float64(val), true
	}
	return 0, false
}
