package audio

// This file adds DSP effects as miniaudio "nodes" and the plumbing to splice a
// chain of them into the graph between a group and its parent.
//
// Signal flow without FX:        group ─────────────────────────► parent
// Signal flow with an FX chain:  group ─► fx[0] ─► fx[1] ─► ... ─► parent
//
// miniaudio ships ready-made filter nodes (low-shelf, peak, high-shelf, delay),
// so an EQ is just three of those wired in series, and a simple reverb is a
// delay node with feedback. Each node is a ma_node under the hood, which is why
// the same attach/detach calls work for groups, sounds, and effects alike — a
// sound group is itself a node (its first struct member is ma_node_base, so a
// ma_sound_group* can be cast straight to ma_node*).

/*
#include <stdlib.h>
#include "miniaudio.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// shelfQ is the "quality factor" for the shelving/peak filters. ~0.707 is the
// Butterworth value: a smooth response with no resonant bump. The project TOML
// doesn't expose Q, so we use this for every band.
const shelfQ = 0.70710678

// cleanup pairs a node (to uninit) with the heap block backing it (to free).
// Node pointers are unsafe.Pointer because miniaudio declares `typedef void
// ma_node;`, so cgo maps every ma_node* parameter to unsafe.Pointer.
type cleanup struct {
	node unsafe.Pointer
	mem  unsafe.Pointer
}

// Effect is one unit in an FX chain. It may be backed by several miniaudio
// nodes internally (the EQ is three), but it presents a single inNode (where
// upstream audio enters) and outNode (what feeds the next stage).
type Effect struct {
	kind       string
	channels   C.ma_uint32
	sampleRate C.ma_uint32

	inNode  unsafe.Pointer // where upstream audio enters this effect
	outNode unsafe.Pointer // what feeds the next stage

	// typed handles kept so we can reconfigure parameters at runtime.
	loshelf *C.ma_loshelf_node
	peak    *C.ma_peak_node
	hishelf *C.ma_hishelf_node
	delay   *C.ma_delay_node

	// cached EQ params — reinit needs the *whole* band (freq+gain+Q) even when
	// only one value changed, so we remember them all.
	lowHz, lowDB   float64
	midHz, midDB   float64
	highHz, highDB float64

	cleanups []cleanup // freed in reverse on Close
}

// Kind reports the effect type ("eq_3band", "reverb", ...).
func (ef *Effect) Kind() string { return ef.kind }

// ─────────────────────────────────────────────
//  EQ (3-band)
// ─────────────────────────────────────────────

// NewEQ3Band builds low-shelf → peak → high-shelf in series. Low/high are
// shelves (boost/cut everything below/above their frequency); mid is a bell.
func (e *Engine) NewEQ3Band(lowHz, lowDB, midHz, midDB, highHz, highDB float64) (*Effect, error) {
	graph := C.ma_engine_get_node_graph(e.ptr)
	ch := C.ma_engine_get_channels(e.ptr)
	sr := C.ma_engine_get_sample_rate(e.ptr)

	ef := &Effect{kind: "eq_3band", channels: ch, sampleRate: sr,
		lowHz: lowHz, lowDB: lowDB, midHz: midHz, midDB: midDB, highHz: highHz, highDB: highDB}

	// low shelf
	lo := (*C.ma_loshelf_node)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_loshelf_node{}))))
	loCfg := C.ma_loshelf_node_config_init(ch, sr, C.double(lowDB), C.double(shelfQ), C.double(lowHz))
	if res := C.ma_loshelf_node_init(graph, &loCfg, nil, lo); res != C.MA_SUCCESS {
		C.free(unsafe.Pointer(lo))
		return nil, fmt.Errorf("audio: eq low-shelf init: %s", maResult(res))
	}
	ef.loshelf = lo
	ef.cleanups = append(ef.cleanups, cleanup{unsafe.Pointer(lo), unsafe.Pointer(lo)})

	// mid peak
	mid := (*C.ma_peak_node)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_peak_node{}))))
	midCfg := C.ma_peak_node_config_init(ch, sr, C.double(midDB), C.double(shelfQ), C.double(midHz))
	if res := C.ma_peak_node_init(graph, &midCfg, nil, mid); res != C.MA_SUCCESS {
		ef.Close()
		C.free(unsafe.Pointer(mid))
		return nil, fmt.Errorf("audio: eq mid-peak init: %s", maResult(res))
	}
	ef.peak = mid
	ef.cleanups = append(ef.cleanups, cleanup{unsafe.Pointer(mid), unsafe.Pointer(mid)})

	// high shelf
	hi := (*C.ma_hishelf_node)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_hishelf_node{}))))
	hiCfg := C.ma_hishelf_node_config_init(ch, sr, C.double(highDB), C.double(shelfQ), C.double(highHz))
	if res := C.ma_hishelf_node_init(graph, &hiCfg, nil, hi); res != C.MA_SUCCESS {
		ef.Close()
		C.free(unsafe.Pointer(hi))
		return nil, fmt.Errorf("audio: eq high-shelf init: %s", maResult(res))
	}
	ef.hishelf = hi
	ef.cleanups = append(ef.cleanups, cleanup{unsafe.Pointer(hi), unsafe.Pointer(hi)})

	// wire the three in series: low → mid → high
	loNode := unsafe.Pointer(lo)
	midNode := unsafe.Pointer(mid)
	hiNode := unsafe.Pointer(hi)
	if err := attach(loNode, midNode); err != nil {
		ef.Close()
		return nil, err
	}
	if err := attach(midNode, hiNode); err != nil {
		ef.Close()
		return nil, err
	}

	ef.inNode = loNode  // audio enters at the low shelf
	ef.outNode = hiNode // and leaves at the high shelf
	return ef, nil
}

// ─────────────────────────────────────────────
//  Reverb (delay-node approximation)
// ─────────────────────────────────────────────

// NewReverb fakes a reverb with a single feedback delay. roomSize (0..1) maps
// to delay length and feedback amount; wet (0..1) is how much effect is mixed
// in. It's an echo, not a true reverb, but it's audible and parameter-driven.
func (e *Engine) NewReverb(roomSize, wet float64) (*Effect, error) {
	graph := C.ma_engine_get_node_graph(e.ptr)
	ch := C.ma_engine_get_channels(e.ptr)
	sr := C.ma_engine_get_sample_rate(e.ptr)

	ef := &Effect{kind: "reverb", channels: ch, sampleRate: sr}

	delayFrames := C.ma_uint32((0.02 + clamp01(roomSize)*0.18) * float64(sr)) // 20–200 ms
	cfg := C.ma_delay_node_config_init(ch, sr, delayFrames, C.float(reverbDecay(roomSize)))
	cfg.delay.wet = C.float(clamp01(wet))
	cfg.delay.dry = 1.0

	d := (*C.ma_delay_node)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_delay_node{}))))
	if res := C.ma_delay_node_init(graph, &cfg, nil, d); res != C.MA_SUCCESS {
		C.free(unsafe.Pointer(d))
		return nil, fmt.Errorf("audio: reverb init: %s", maResult(res))
	}
	ef.delay = d
	ef.cleanups = append(ef.cleanups, cleanup{unsafe.Pointer(d), unsafe.Pointer(d)})

	node := unsafe.Pointer(d)
	ef.inNode = node
	ef.outNode = node
	return ef, nil
}

// ─────────────────────────────────────────────
//  Live parameter updates
// ─────────────────────────────────────────────

// SetParam updates one parameter at runtime (driven by EventFXParamChanged).
// Unknown keys are ignored. EQ bands are reconfigured via *_node_reinit; the
// delay node has dedicated setters.
func (ef *Effect) SetParam(key string, value float64) {
	switch ef.kind {
	case "eq_3band":
		switch key {
		case "low_hz":
			ef.lowHz = value
			ef.reinitLow()
		case "low_db":
			ef.lowDB = value
			ef.reinitLow()
		case "mid_hz":
			ef.midHz = value
			ef.reinitMid()
		case "mid_db":
			ef.midDB = value
			ef.reinitMid()
		case "high_hz":
			ef.highHz = value
			ef.reinitHigh()
		case "high_db":
			ef.highDB = value
			ef.reinitHigh()
		}
	case "reverb":
		switch key {
		case "wet":
			C.ma_delay_node_set_wet(ef.delay, C.float(clamp01(value)))
		case "room_size":
			// Delay length can't change without a rebuild, so we approximate a
			// bigger room by increasing feedback only.
			C.ma_delay_node_set_decay(ef.delay, C.float(reverbDecay(value)))
		}
	}
}

func (ef *Effect) reinitLow() {
	cfg := C.ma_loshelf_node_config_init(ef.channels, ef.sampleRate, C.double(ef.lowDB), C.double(shelfQ), C.double(ef.lowHz))
	C.ma_loshelf_node_reinit(&cfg.loshelf, ef.loshelf)
}

func (ef *Effect) reinitMid() {
	cfg := C.ma_peak_node_config_init(ef.channels, ef.sampleRate, C.double(ef.midDB), C.double(shelfQ), C.double(ef.midHz))
	C.ma_peak_node_reinit(&cfg.peak, ef.peak)
}

func (ef *Effect) reinitHigh() {
	cfg := C.ma_hishelf_node_config_init(ef.channels, ef.sampleRate, C.double(ef.highDB), C.double(shelfQ), C.double(ef.highHz))
	C.ma_hishelf_node_reinit(&cfg.hishelf, ef.hishelf)
}

// Close uninitialises and frees every node backing the effect, newest first.
func (ef *Effect) Close() {
	for i := len(ef.cleanups) - 1; i >= 0; i-- {
		c := ef.cleanups[i]
		C.ma_node_uninit(c.node, nil)
		C.free(c.mem)
	}
	ef.cleanups = nil
}

// ─────────────────────────────────────────────
//  Chain wiring
// ─────────────────────────────────────────────

// AttachChain wires src → fx[0] → ... → fx[n] → dst. A nil dst means the chain
// feeds the engine endpoint (the speakers) — used for the master chain.
// Attaching an output bus that's already connected replaces the old
// connection, so this also correctly *re-routes* an existing group.
func (e *Engine) AttachChain(src *Group, fx []*Effect, dst *Group) error {
	srcNode := unsafe.Pointer(src.ptr)

	var dstNode unsafe.Pointer
	if dst != nil {
		dstNode = unsafe.Pointer(dst.ptr)
	} else {
		dstNode = C.ma_engine_get_endpoint(e.ptr)
	}

	if len(fx) == 0 {
		return attach(srcNode, dstNode)
	}
	if err := attach(srcNode, fx[0].inNode); err != nil {
		return err
	}
	for i := 0; i+1 < len(fx); i++ {
		if err := attach(fx[i].outNode, fx[i+1].inNode); err != nil {
			return err
		}
	}
	return attach(fx[len(fx)-1].outNode, dstNode)
}

// attach connects out's bus 0 to in's bus 0.
func attach(out, in unsafe.Pointer) error {
	if res := C.ma_node_attach_output_bus(out, 0, in, 0); res != C.MA_SUCCESS {
		return fmt.Errorf("audio: attach node: %s", maResult(res))
	}
	return nil
}

// ─────────────────────────────────────────────
//  small helpers
// ─────────────────────────────────────────────

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// reverbDecay maps roomSize (0..1) to a feedback amount, capped below 1.0 so
// the delay always dies out instead of ringing forever.
func reverbDecay(roomSize float64) float64 {
	return clamp01(roomSize) * 0.6
}
