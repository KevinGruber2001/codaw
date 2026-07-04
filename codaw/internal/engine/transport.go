package engine

// transport converts musical time into audio time.
//
// Musicians think in *beats*; audio hardware thinks in *sample frames*. A clip
// that starts on "beat 8" needs to become "sample frame N" before the engine
// can schedule it. That conversion depends on two numbers:
//
//   - sampleRate: how many frames the device plays per second (e.g. 48000).
//   - bpm:        beats per minute, i.e. the tempo.
//
// From those:
//
//	beats per second   = bpm / 60
//	frames per second  = sampleRate
//	frames per beat    = sampleRate / (bpm / 60) = sampleRate * 60 / bpm
//
// Example: 48000 Hz at 120 BPM → 48000 * 60 / 120 = 24000 frames per beat,
// i.e. each beat lasts half a second. Beat 8 → 192000 frames → 4 seconds in.
//
// Note we use the *device* sample rate, not the project's requested rate.
// miniaudio resamples decoded files to the device rate, and the global clock we
// schedule against ticks at the device rate, so all timing math must too.
type transport struct {
	sampleRate int
	bpm        float64
}

// framesPerBeat is the heart of the conversion (see the package comment above).
func (t transport) framesPerBeat() float64 {
	return float64(t.sampleRate) * 60.0 / t.bpm
}

// beatsToFrames converts a beat position into a number of sample frames.
// Used to turn a clip's Start/End (in beats) into offsets on the global clock.
func (t transport) beatsToFrames(beats float64) uint64 {
	if beats <= 0 {
		return 0
	}
	return uint64(beats * t.framesPerBeat())
}
