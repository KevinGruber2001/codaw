// Package engine plays a project and keeps it in step with its files. It is a coordinator: the
// mixer (internal/mix) makes the audio, the player (internal/audio) sends it to the sound card, and
// the store (internal/state) says what the project currently is.
//
//	store ── snapshot ──► mixer ◄── MixBlock ── player ──► device
//	                        ▲
//	        Play/Stop/Seek ─┘
//
// When a file is saved, the watcher puts a new snapshot in the store; the engine decodes any new
// samples and hands the snapshot to the mixer, which plays it from its next block. Nothing is
// rebuilt and nothing has to be told what changed.
package engine

import (
	"log"

	"github.com/KevinGruber2001/codaw/internal/audio"
	"github.com/KevinGruber2001/codaw/internal/mix"
	"github.com/KevinGruber2001/codaw/internal/sample"
	"github.com/KevinGruber2001/codaw/internal/state"
)

// Engine plays a project and keeps it in sync with the Store.
type Engine struct {
	store  *state.Store
	cache  *sample.Cache
	mixer  *mix.Mixer
	player *audio.Player // nil for an offline engine

	events      chan state.Event
	unsubscribe func()
	listenDone  chan struct{}
}

// New opens the audio device and starts following the store. Call Load next, then Play.
//
// The engine runs at the project's sample rate, live and when rendering alike; if the device runs at
// another rate, the device layer converts. Changing the project's rate needs a restart.
func New(store *state.Store) (*Engine, error) {
	e := newEngine(store)
	player, err := audio.NewPlayer(e.mixer.Rate(), mix.Channels, e.mixer.MixBlock)
	if err != nil {
		return nil, err
	}
	e.player = player
	e.events = make(chan state.Event, 16)
	e.listenDone = make(chan struct{})
	e.unsubscribe = store.Subscribe(e.events)
	go e.listen()
	return e, nil
}

// NewOffline makes an engine with no audio device, for Render. It does not follow the store.
func NewOffline(store *state.Store) (*Engine, error) {
	return newEngine(store), nil
}

func newEngine(store *state.Store) *Engine {
	rate := store.Get().Transport.SampleRate
	cache := sample.NewCache(rate)
	return &Engine{store: store, cache: cache, mixer: mix.New(rate, cache)}
}

// Load hands the store's current project to the mixer, after decoding its samples. An error says a
// sample could not be read; the project plays anyway, without it.
func (e *Engine) Load() error {
	p := e.store.Get()
	err := mix.Preload(p, e.load)
	e.mixer.Publish(p)
	return err
}

func (e *Engine) load(path string) error {
	_, err := e.cache.Load(path)
	return err
}

// listen hands every new snapshot to the mixer. Samples are decoded here, so the mixing goroutine
// only ever finds them in the cache.
func (e *Engine) listen() {
	defer close(e.listenDone)
	for range e.events {
		p := e.store.Get()
		if p.Transport.SampleRate != e.mixer.Rate() {
			log.Printf("[engine] sample_rate is now %d; the engine keeps %d until it restarts", p.Transport.SampleRate, e.mixer.Rate())
		}
		if err := mix.Preload(p, e.load); err != nil {
			log.Printf("[engine] %v", err)
		}
		e.mixer.Publish(p)
	}
}

// Play starts playback where the playhead is.
func (e *Engine) Play() error {
	e.mixer.Play()
	return nil
}

// Stop pauses: the playhead stays where the sound stopped, and Play carries on from there. Seek(0)
// goes back to the start.
func (e *Engine) Stop() {
	at := e.PositionBeats()
	e.mixer.Stop()
	e.mixer.Seek(at)
}

// Seek moves the playhead to a beat, playing or not.
func (e *Engine) Seek(beat float64) error {
	e.mixer.Seek(beat)
	return nil
}

// PositionBeats is the beat being heard. The mixer is ahead of the speakers by what waits in the
// player's buffer, so that is taken off while playing.
func (e *Engine) PositionBeats() float64 {
	beat := e.mixer.Position()
	if e.player != nil && e.mixer.Playing() {
		ahead := float64(e.player.Buffered()) / float64(e.mixer.Rate()) * e.store.Get().Transport.BPM / 60
		beat = max(0, beat-ahead)
	}
	return beat
}

// Playing says whether the transport runs.
func (e *Engine) Playing() bool { return e.mixer.Playing() }

// Close stops following the store and closes the device. The Engine must not be used afterwards.
func (e *Engine) Close() {
	if e.unsubscribe != nil {
		e.unsubscribe()
		close(e.events)
		<-e.listenDone
		e.unsubscribe = nil
	}
	if e.player != nil {
		e.player.Close()
		e.player = nil
	}
}
