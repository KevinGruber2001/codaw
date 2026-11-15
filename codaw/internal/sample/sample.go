// Package sample loads audio files into plain Go slices.
//
// Everything downstream — the mixer, effects, waveform drawing, recording —
// works on Buffer. Nothing else in CodaW should know what a WAV header looks
// like, what bit depth a file was stored at, or that sample rates can differ.
package sample

import (
	"fmt"
	"os"

	"github.com/go-audio/wav"
)

// Buffer is decoded audio, ready to mix: interleaved float32 in [-1, 1].
type Buffer struct {
	// stored interleaved (L,R,L,R), so frame n lives at
	// Data[n*Channels] .. Data[n*Channels+Channels-1]
	Data []float32

	// number of channels, if mono = 1, if stereo = 2
	Channels int

	// rate of samples per second
	SampleRate int
}

// Frames is the number of sample-frames (one sample per channel) in the buffer.
// This is what you want for anything time-related; len(Data) is not.
func (b *Buffer) Frames() int {
	if b.Channels <= 0 {
		return 0
	}
	return len(b.Data) / b.Channels
}

// Seconds is the buffer's duration.
func (b *Buffer) Seconds() float64 {
	if b.SampleRate <= 0 {
		return 0
	}
	return float64(b.Frames()) / float64(b.SampleRate)
}

// Load decodes an audio file and returns it at the requested sample rate,
// resampling once if the file doesn't already match.
//
// Resampling happens HERE and nowhere else. After Load, every Buffer in the
// program is at the project's rate, so the mixer never has to think about rate
// conversion at all. That costs a few milliseconds and some memory per file,
// once, and removes an entire category of complexity from the hot path.
func Load(path string, rate int) (*Buffer, error) {
	b, err := decodeWav(path)
	if err != nil {
		return nil, err
	}
	if b.SampleRate != rate {
		return resampleLinear(b, rate), nil
	}
	return b, nil
}

// decodeWav reads a PCM WAV file into a Buffer at the file's own sample rate.
// Callers go through Load, which handles rate conversion.
//
// We lean on go-audio/wav for the file format itself: WAV has more variants
// than it looks like (24-bit packed samples, float encodings, metadata chunks
// sitting between the header and the audio) and that library already handles
// them. What we add on top is a narrow contract — integer PCM, 16- or 24-bit,
// mono or stereo — and a clear error for anything else.
func decodeWav(path string) (*Buffer, error) {

	// handle opening the file
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sample: %w", err) // os errors already name the path
	}
	defer f.Close()

	// Reading info, for fast rejection if neccesary. ReadInfo parses the header
	// only — a few dozen bytes — so a file we cannot handle costs us nothing.
	d := wav.NewDecoder(f)
	d.ReadInfo()
	if err := d.Err(); err != nil {
		return nil, fmt.Errorf("sample: read %s: %w", path, err)
	}
	if !d.IsValidFile() {
		return nil, fmt.Errorf("sample: %s is not a valid WAV file", path)
	}

	// WavAudioFormat says how the samples are encoded: 1 = signed integers,
	// 3 = float32 already, 0xFFFE = "extensible" (real format in a sub-header).
	// The decoder will read a format-3 file as integers and hand back noise, so
	// this check is doing real work.
	if d.WavAudioFormat != 1 {
		return nil, fmt.Errorf("sample: %s: unsupported WAV encoding (format %d, want 1 = integer PCM)", path, d.WavAudioFormat)
	}

	switch d.BitDepth {
	case 16, 24:
	default:
		return nil, fmt.Errorf("sample: %s: unsupported bit depth %d (want 16 or 24)", path, d.BitDepth)
	}

	if d.NumChans != 1 && d.NumChans != 2 {
		return nil, fmt.Errorf("sample: %s: unsupported channel count %d (want mono or stereo)", path, d.NumChans)
	}

	// decode channel. FullPCMBuffer decodes the whole file into memory as ints,
	// already interleaved, which is the layout Buffer wants.
	//
	// It costs two allocations: an []int (8 bytes per sample) that we throw away
	// and the []float32 we keep, so a 4-minute stereo 48kHz file peaks around
	// 180MB before settling at ~90MB. Acceptable while samples are one-shots and
	// short loops; if it ever isn't, d.PCMBuffer() fills a reusable buffer in
	// chunks instead, at the cost of a decode loop here.
	ints, err := d.FullPCMBuffer()
	if err != nil {
		return nil, fmt.Errorf("sample: decode %s: %w", path, err)
	}
	if len(ints.Data) == 0 {
		return nil, fmt.Errorf("sample: %s contains no audio", path)
	}

	// The samples are integers scaled to the file's bit depth: 16-bit runs
	// -32768..32767, 24-bit runs -8388608..8388607. AsFloat32Buffer divides by
	// 2^(bits-1), giving [-1, 1] regardless of the source depth. After this one
	// normalisation nothing downstream needs to know how the file was stored.
	floats := ints.AsFloat32Buffer()

	return &Buffer{
		Data:       floats.Data,
		Channels:   ints.Format.NumChannels,
		SampleRate: ints.Format.SampleRate,
	}, nil
}

// resampleLinear converts to a new sample rate by straight-line interpolation
// between neighbouring frames.
//
// Linear interpolation is not a good resampler — it rolls off the top end and
// adds a little aliasing. We use it anyway because it runs ONCE per file at
// load time, it is about twenty lines, and you can see exactly what it does. If
// a sample ever audibly suffers, the fix is to swap this function for a
// windowed-sinc one; the signature stays the same and nothing else in the
// codebase has to change.
func resampleLinear(in *Buffer, rate int) *Buffer {
	ch := in.Channels
	inFrames := in.Frames()

	// Nothing sensible to interpolate to.
	if rate <= 0 || in.SampleRate <= 0 || rate == in.SampleRate {
		return in
	}
	// Nothing to interpolate between. Past this point every return is at rate,
	// so callers can rely on that without checking.
	if inFrames == 0 {
		return &Buffer{Channels: ch, SampleRate: rate}
	}
	if inFrames == 1 {
		out := make([]float32, ch)
		copy(out, in.Data[:ch])
		return &Buffer{Data: out, Channels: ch, SampleRate: rate}
	}

	// ratio is input frames consumed per output frame. It maps an output index
	// back to a (fractional) input index, which is why it is in/out and not the
	// other way round: 44100 -> 48000 gives 0.91875, so each output frame
	// advances less than one input frame. That is what upsampling means.
	ratio := float64(in.SampleRate) / float64(rate)

	outFrames := int(float64(inFrames) / ratio)
	if outFrames < 1 {
		outFrames = 1
	}

	out := make([]float32, outFrames*ch)
	for i := 0; i < outFrames; i++ {
		// Recomputed from i every iteration rather than accumulated with
		// src += ratio: ratio is not exactly representable in binary, and an
		// accumulated error compounds into audible drift over a long file.
		src := float64(i) * ratio

		i0 := int(src) // frame before
		if i0 > inFrames-1 {
			i0 = inFrames - 1
		}
		i1 := i0 + 1 // frame after
		if i1 > inFrames-1 {
			i1 = inFrames - 1 // last frame holds rather than reading past the end
		}
		frac := float32(src - float64(i0))

		// i0 and i1 are FRAME indices. Each channel is interpolated against the
		// same channel — lerping Data[n] against Data[n+1] on a stereo buffer
		// would blend left into right.
		for c := 0; c < ch; c++ {
			a := in.Data[i0*ch+c]
			b := in.Data[i1*ch+c]
			out[i*ch+c] = a + (b-a)*frac
		}
	}

	return &Buffer{Data: out, Channels: ch, SampleRate: rate}
}
