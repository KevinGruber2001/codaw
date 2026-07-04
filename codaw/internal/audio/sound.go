package audio

// This file adds the two building blocks the engine arranges into a graph:
//
//   Group  — a miniaudio "sound group": a mixing node with its own gain + pan.
//            Groups can be parented to other groups, so we build the routing
//            tree clip → track → bus → master out of them. A group sums
//            everything attached to it and applies its volume/pan, then passes
//            the result to its parent. miniaudio does the actual mixing.
//
//   Sound  — a decoded audio file attached to a group. This is a clip: it has
//            its own gain, can loop, and can be scheduled to start/stop at an
//            exact sample position on the engine's global clock.
//
// Everything here stays in package audio because it touches C types, and cgo
// types cannot cross package boundaries.

/*
#include <stdlib.h>
#include "miniaudio.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// ─────────────────────────────────────────────
//  Group
// ─────────────────────────────────────────────

// Group wraps ma_sound_group. Create one with Engine.NewGroup, set its gain/pan,
// and attach Sounds (or child Groups) to it. Always pair with Close.
type Group struct {
	ptr *C.ma_sound_group
}

// NewGroup creates a mixing node attached to parent. Pass nil for parent to
// attach directly to the engine's output (the master endpoint / speakers).
//
// Routing in CodaW is just nested groups:
//
//	NewGroup(nil)          // master  → speakers
//	NewGroup(master)       // a bus   → master
//	NewGroup(bus or master)// a track → bus (or straight to master)
func (e *Engine) NewGroup(parent *Group) (*Group, error) {
	ptr := (*C.ma_sound_group)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_sound_group{}))))
	if ptr == nil {
		return nil, fmt.Errorf("audio: out of memory allocating group")
	}

	var parentPtr *C.ma_sound_group
	if parent != nil {
		parentPtr = parent.ptr
	}

	// flags = 0 → default group behaviour. parentPtr nil → engine endpoint.
	if res := C.ma_sound_group_init(e.ptr, 0, parentPtr, ptr); res != C.MA_SUCCESS {
		C.free(unsafe.Pointer(ptr))
		return nil, fmt.Errorf("audio: ma_sound_group_init failed: %s", maResult(res))
	}
	return &Group{ptr: ptr}, nil
}

// SetGainDB sets the group's volume from a decibel value (0 dB = unity).
// miniaudio mixes in linear amplitude, so we convert: dB → linear.
func (g *Group) SetGainDB(db float64) {
	C.ma_sound_group_set_volume(g.ptr, C.ma_volume_db_to_linear(C.float(db)))
}

// SetVolume sets the raw linear volume (0.0 = silent, 1.0 = unity). Used for
// hard mute, where dB has no clean representation for "off".
func (g *Group) SetVolume(v float64) {
	C.ma_sound_group_set_volume(g.ptr, C.float(v))
}

// SetPan positions the group in the stereo field: -1 left, 0 center, +1 right.
func (g *Group) SetPan(pan float64) {
	C.ma_sound_group_set_pan(g.ptr, C.float(pan))
}

// Close uninitialises and frees the group. Detach/close child sounds first.
func (g *Group) Close() {
	if g.ptr == nil {
		return
	}
	C.ma_sound_group_uninit(g.ptr)
	C.free(unsafe.Pointer(g.ptr))
	g.ptr = nil
}

// ─────────────────────────────────────────────
//  Sound
// ─────────────────────────────────────────────

// Sound wraps ma_sound: one decoded audio file attached to a group. In CodaW
// terms, a Sound is a clip on a track.
type Sound struct {
	ptr *C.ma_sound
}

// NewSound decodes a file fully into memory (MA_SOUND_FLAG_DECODE) and attaches
// it to group. Decoding up front — rather than streaming from disk — keeps the
// audio thread from doing file I/O, which is what you want for short samples
// that may loop. The sound does not play until you call Start.
func (e *Engine) NewSound(path string, group *Group) (*Sound, error) {
	ptr := (*C.ma_sound)(C.malloc(C.size_t(unsafe.Sizeof(C.ma_sound{}))))
	if ptr == nil {
		return nil, fmt.Errorf("audio: out of memory allocating sound")
	}

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var groupPtr *C.ma_sound_group
	if group != nil {
		groupPtr = group.ptr
	}

	if res := C.ma_sound_init_from_file(e.ptr, cpath, C.MA_SOUND_FLAG_DECODE, groupPtr, nil, ptr); res != C.MA_SUCCESS {
		C.free(unsafe.Pointer(ptr))
		return nil, fmt.Errorf("audio: failed to load %q: %s", path, maResult(res))
	}
	return &Sound{ptr: ptr}, nil
}

// SetGainDB sets the clip's own volume trim in dB (applied before the group's).
func (s *Sound) SetGainDB(db float64) {
	C.ma_sound_set_volume(s.ptr, C.ma_volume_db_to_linear(C.float(db)))
}

// SetLooping controls whether the sound restarts from the beginning when it
// reaches the end of the decoded file.
func (s *Sound) SetLooping(loop bool) {
	C.ma_sound_set_looping(s.ptr, boolToMA(loop))
}

// SeekToSecond moves the playback cursor to the given offset (in seconds) into
// the source file. Used for clip in-points (trim/split). Note: if the sound
// also loops, miniaudio loops back to the start of the file, not this offset.
func (s *Sound) SeekToSecond(sec float64) error {
	if res := C.ma_sound_seek_to_second(s.ptr, C.float(sec)); res != C.MA_SUCCESS {
		return fmt.Errorf("audio: seek to %.3fs failed: %s", sec, maResult(res))
	}
	return nil
}

// ScheduleStart tells the sound to begin outputting audio when the engine's
// global clock reaches the given absolute frame. Combined with the engine's
// current time, this is how we place clips on the timeline.
func (s *Sound) ScheduleStart(absoluteFrame uint64) {
	C.ma_sound_set_start_time_in_pcm_frames(s.ptr, C.ma_uint64(absoluteFrame))
}

// ScheduleStop tells the sound to stop outputting at the given absolute frame.
// For a looping clip this is what bounds the loop to its [start,end] window.
func (s *Sound) ScheduleStop(absoluteFrame uint64) {
	C.ma_sound_set_stop_time_in_pcm_frames(s.ptr, C.ma_uint64(absoluteFrame))
}

// Start arms the sound. With a scheduled start time it stays silent until the
// clock reaches it; with no schedule it plays immediately.
func (s *Sound) Start() error {
	if res := C.ma_sound_start(s.ptr); res != C.MA_SUCCESS {
		return fmt.Errorf("audio: ma_sound_start failed: %s", maResult(res))
	}
	return nil
}

// Stop halts playback immediately.
func (s *Sound) Stop() error {
	if res := C.ma_sound_stop(s.ptr); res != C.MA_SUCCESS {
		return fmt.Errorf("audio: ma_sound_stop failed: %s", maResult(res))
	}
	return nil
}

// Close uninitialises and frees the sound.
func (s *Sound) Close() {
	if s.ptr == nil {
		return
	}
	C.ma_sound_uninit(s.ptr)
	C.free(unsafe.Pointer(s.ptr))
	s.ptr = nil
}

// ─────────────────────────────────────────────
//  Engine clock
// ─────────────────────────────────────────────

// TimeFrames returns the engine's current position on its global clock, counted
// in sample frames since the engine started. Scheduling is done relative to
// this value (e.g. "start this clip 48000 frames from now" = 1 second at 48kHz).
func (e *Engine) TimeFrames() uint64 {
	return uint64(C.ma_engine_get_time_in_pcm_frames(e.ptr))
}

// boolToMA converts a Go bool to miniaudio's ma_bool32 (1 = true, 0 = false).
func boolToMA(b bool) C.ma_bool32 {
	if b {
		return C.ma_bool32(1)
	}
	return C.ma_bool32(0)
}
