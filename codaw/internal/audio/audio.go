// Package audio is a thin cgo wrapper around miniaudio (https://miniaud.io).
//
// miniaudio gives us, in one C header: device output (Core Audio on macOS,
// WASAPI on Windows, ALSA/PulseAudio on Linux), file decoding (WAV/FLAC/MP3),
// and a node graph for mixing/effects. This package exposes just enough of it
// in idiomatic Go to start the audio device and play a sound file. Higher-level
// concepts (tracks, buses, the project graph) live in internal/engine and build
// on top of this.
package audio

/*
// --- cgo preamble -----------------------------------------------------------
// Everything inside this comment block is C, parsed by cgo. The directives tell
// the C compiler/linker how to build the bits of C this package touches.

// -I${SRCDIR} lets #include "miniaudio.h" resolve to the vendored copy sitting
// next to this file. ${SRCDIR} is expanded by cgo to this package's directory.
#cgo CFLAGS: -I${SRCDIR}

// miniaudio talks to the OS through native audio frameworks. On macOS those are
// system frameworks we link against. (Linux/Windows get their own lines when we
// build there; cgo only applies the line matching the target OS.)
#cgo darwin  LDFLAGS: -framework CoreFoundation -framework CoreAudio -framework AudioToolbox
#cgo linux   LDFLAGS: -lm -lpthread -ldl
#cgo windows LDFLAGS: -lole32 -lwinmm

#include <stdlib.h>      // for C.free
#include "miniaudio.h"   // declarations only; the implementation is in bridge.c
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Engine wraps a miniaudio ma_engine: a running audio device plus an internal
// node graph that mixes everything routed into it and streams it to the
// speakers. Create one with NewEngine and always pair it with Close.
type Engine struct {
	// We keep the ma_engine on the C heap, not as a Go value, on purpose.
	// ma_engine is a large struct full of internal C pointers. cgo forbids
	// passing a Go pointer that itself contains C pointers across the boundary,
	// and the engine also spins up a background thread that keeps referencing
	// this memory. Allocating it with C.malloc keeps it out of reach of Go's
	// garbage collector and at a stable address for the engine's lifetime.
	ptr *C.ma_engine
}

// NewEngine starts the default audio device with miniaudio's defaults
// (typically 2 channels at the device's native sample rate). The engine begins
// running immediately; sounds played on it are audible right away.
func NewEngine() (*Engine, error) {
	ptr := (*C.ma_engine)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_engine{}))))
	if ptr == nil {
		return nil, fmt.Errorf("audio: out of memory allocating engine")
	}

	// Passing nil for the config means "use sensible defaults".
	if res := C.ma_engine_init(nil, ptr); res != C.MA_SUCCESS {
		C.free(unsafe.Pointer(ptr))
		return nil, fmt.Errorf("audio: ma_engine_init failed: %s", maResult(res))
	}
	return &Engine{ptr: ptr}, nil
}

// PlayFile decodes and plays a sound file (WAV/FLAC/MP3) on the engine. It is
// fire-and-forget: the call returns immediately and the sound plays to
// completion on the engine's audio thread. Good enough to prove the pipeline
// works; richer playback (looping, seeking, per-sound gain) comes later via a
// Sound type.
func (e *Engine) PlayFile(path string) error {
	cpath := C.CString(path) // Go string -> C string (heap-allocated, must free)
	defer C.free(unsafe.Pointer(cpath))

	if res := C.ma_engine_play_sound(e.ptr, cpath, nil); res != C.MA_SUCCESS {
		return fmt.Errorf("audio: failed to play %q: %s", path, maResult(res))
	}
	return nil
}

// SampleRate reports the sample rate (Hz) the engine's device is actually
// running at. Useful later when converting bpm/seconds into sample counts.
func (e *Engine) SampleRate() uint32 {
	return uint32(C.ma_engine_get_sample_rate(e.ptr))
}

// Close stops the audio device and frees the engine. Safe to call once; after
// Close the Engine must not be used again.
func (e *Engine) Close() {
	if e.ptr == nil {
		return
	}
	C.ma_engine_uninit(e.ptr)
	C.free(unsafe.Pointer(e.ptr))
	e.ptr = nil
}

// maResult turns a miniaudio result code into a human-readable string so error
// messages say "A file could not be opened" instead of "-3".
func maResult(res C.ma_result) string {
	return C.GoString(C.ma_result_description(res))
}
