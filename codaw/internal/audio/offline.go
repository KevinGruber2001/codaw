package audio

// Offline mode: an engine with no audio device. Instead of a background thread
// streaming to the speakers, *you* pull rendered audio out of the node graph
// frame-by-frame with ReadFrames. Same graph, same mixing, same effects — only
// the output destination differs. This is what offline WAV export is built on.

/*
#include <stdlib.h>
#include "miniaudio.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// NewOfflineEngine creates a device-less engine fixed at the given sample rate
// and channel count. Build the graph on it exactly as with NewEngine, then call
// ReadFrames in a loop to render.
func NewOfflineEngine(sampleRate, channels uint32) (*Engine, error) {
	cfg := C.ma_engine_config_init()
	cfg.noDevice = C.ma_bool32(1)        // don't open a playback device
	cfg.channels = C.ma_uint32(channels) // must be set explicitly with no device
	cfg.sampleRate = C.ma_uint32(sampleRate)

	ptr := (*C.ma_engine)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_engine{}))))
	if ptr == nil {
		return nil, fmt.Errorf("audio: out of memory allocating offline engine")
	}
	if res := C.ma_engine_init(&cfg, ptr); res != C.MA_SUCCESS {
		C.free(unsafe.Pointer(ptr))
		return nil, fmt.Errorf("audio: offline ma_engine_init failed: %s", maResult(res))
	}
	return &Engine{ptr: ptr}, nil
}

// Channels reports the engine's output channel count.
func (e *Engine) Channels() uint32 {
	return uint32(C.ma_engine_get_channels(e.ptr))
}

// ReadFrames renders the next chunk of audio into out (interleaved float32, the
// engine's native format) and returns the number of frames actually produced.
// out must be sized framesWanted*channels. Reading is what advances the
// engine's clock offline, so scheduled clips trigger as frames are pulled.
func (e *Engine) ReadFrames(out []float32, channels uint32) (uint64, error) {
	if len(out) == 0 {
		return 0, nil
	}
	frameCount := C.ma_uint64(uint64(len(out)) / uint64(channels))
	var read C.ma_uint64
	// &out[0] is Go memory, but ma_engine_read_pcm_frames fills it synchronously
	// and doesn't retain the pointer, so passing it across cgo is allowed.
	res := C.ma_engine_read_pcm_frames(e.ptr, unsafe.Pointer(&out[0]), frameCount, &read)
	if res != C.MA_SUCCESS {
		return 0, fmt.Errorf("audio: ma_engine_read_pcm_frames failed: %s", maResult(res))
	}
	return uint64(read), nil
}
