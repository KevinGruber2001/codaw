// Package audio is the only place in CodaW that talks to the sound card. It knows nothing about
// projects, tracks or effects: it moves float32 samples to the default output device, through malgo
// (https://github.com/gen2brain/malgo), a binding of miniaudio's device layer.
package audio

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

const (
	// blockFrames is how much the mixer makes at a time: about 10 ms.
	blockFrames = 512
	// ringFrames is how far ahead of the device the mixer may get: about 85 ms at 48 kHz. That much
	// time can a block take, or a garbage collection pause, before anyone hears a thing. CodaW plays
	// back and renders; it is not played live like an instrument, so the latency costs nothing.
	ringFrames = 8 * blockFrames
)

// Fill makes the next len(buf) samples of interleaved audio. mix.Mixer.MixBlock has this shape.
type Fill func(buf []float32)

// Player streams a Fill to the default output device.
//
// Fill runs on an ordinary goroutine, never on the device's thread. The device callback only copies
// out of a ring buffer that goroutine keeps topped up, so the worst a slow block can do is use up some
// of the ring, not glitch the output.
type Player struct {
	ctx      *malgo.AllocatedContext
	dev      *malgo.Device
	ring     *ring
	channels int

	room      chan struct{} // the callback says it made room in the ring
	stop      chan struct{}
	done      chan struct{}
	underruns atomic.Uint64
}

// NewPlayer opens the default output device at rate, with channels channels, and starts playing
// whatever fill makes. If the device runs at another rate, miniaudio converts.
func NewPlayer(rate, channels int, fill Fill) (*Player, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio: %w", err)
	}
	p := &Player{
		ctx:      ctx,
		ring:     newRing(ringFrames * channels),
		channels: channels,
		room:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}

	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.Playback.Format = malgo.FormatF32
	config.Playback.Channels = uint32(channels)
	config.SampleRate = uint32(rate)
	config.Alsa.NoMMap = 1

	// The device's thread, at a deadline. Nothing here may allocate, lock, log or wait.
	onSamples := func(out, _ []byte, _ uint32) {
		if len(out) == 0 {
			return
		}
		samples := unsafe.Slice((*float32)(unsafe.Pointer(&out[0])), len(out)/4)
		if p.ring.read(samples) {
			p.underruns.Add(1)
		}
		select {
		case p.room <- struct{}{}:
		default: // the mixer has been told already
		}
	}
	p.dev, err = malgo.InitDevice(ctx.Context, config, malgo.DeviceCallbacks{Data: onSamples})
	if err != nil {
		p.free()
		return nil, fmt.Errorf("audio: open the output device: %w", err)
	}

	go p.pump(fill)
	if err := p.dev.Start(); err != nil {
		close(p.stop)
		<-p.done
		p.dev.Uninit()
		p.free()
		return nil, fmt.Errorf("audio: start the output device: %w", err)
	}
	return p, nil
}

// pump keeps the ring full: a block whenever there is room for one, otherwise it waits for the
// callback to make some. It also reports underruns, which the callback may only count.
func (p *Player) pump(fill Fill) {
	defer close(p.done)
	buf := make([]float32, blockFrames*p.channels) // the only allocation, once
	var reported uint64
	lastReport := time.Now()
	for {
		if p.ring.space() >= len(buf) {
			fill(buf)
			p.ring.write(buf)
			continue
		}
		if n := p.underruns.Load(); n != reported && time.Since(lastReport) > time.Second {
			log.Printf("[audio] %d underrun(s): the mixer did not keep up and the device played silence", n-reported)
			reported, lastReport = n, time.Now()
		}
		select {
		case <-p.stop:
			return
		case <-p.room:
		}
	}
}

// Buffered is how many frames are made but not yet handed to the device: how far ahead of what is
// heard the mixer is.
func (p *Player) Buffered() int { return p.ring.buffered() / p.channels }

// Underruns counts the times the device found the ring empty.
func (p *Player) Underruns() uint64 { return p.underruns.Load() }

// Close stops the device and the pump. The Player must not be used afterwards.
func (p *Player) Close() {
	_ = p.dev.Stop()
	close(p.stop)
	<-p.done
	p.dev.Uninit()
	p.free()
}

func (p *Player) free() {
	_ = p.ctx.Uninit()
	p.ctx.Free()
}
