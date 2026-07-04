// Package wav writes 16-bit PCM WAV files. It's deliberately tiny and
// dependency-free: the engine renders float32 samples, and this turns them into
// the most universally playable audio file format.
//
// A WAV file is RIFF-structured: a 12-byte header, a "fmt " chunk describing
// the format, and a "data" chunk holding the raw samples. We write a canonical
// 44-byte header up front with placeholder sizes, stream the samples, then seek
// back and patch the two size fields once we know the total length.
//
//	offset  bytes  field
//	0       4      "RIFF"
//	4       4      file size - 8           (patched on Close)
//	8       4      "WAVE"
//	12      4      "fmt "
//	16      4      16  (fmt chunk size)
//	20      2      1   (PCM)
//	22      2      channels
//	24      4      sample rate
//	28      4      byte rate = rate*channels*2
//	32      2      block align = channels*2
//	34      2      bits per sample = 16
//	36      4      "data"
//	40      4      data size               (patched on Close)
//	44      ...    samples (little-endian int16, interleaved)
package wav

import (
	"bufio"
	"encoding/binary"
	"os"
)

const (
	bitsPerSample  = 16
	bytesPerSample = bitsPerSample / 8
	headerSize     = 44
)

// Writer streams PCM16 samples to a .wav file. Create one with Create, feed it
// float32 samples with WriteFloat32, and always Close it.
type Writer struct {
	f          *os.File
	buf        *bufio.Writer
	sampleRate uint32
	channels   uint16
	dataBytes  uint32
	closed     bool
}

// Create opens path for writing and lays down a placeholder header.
func Create(path string, sampleRate uint32, channels uint16) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := &Writer{f: f, buf: bufio.NewWriter(f), sampleRate: sampleRate, channels: channels}
	if err := w.writeHeader(); err != nil {
		f.Close()
		return nil, err
	}
	return w, nil
}

// writeHeader writes the 44-byte header using the current dataBytes value. With
// dataBytes == 0 (at Create) the size fields are placeholders; Close rewrites
// the header once the real length is known.
func (w *Writer) writeHeader() error {
	var h [headerSize]byte
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+w.dataBytes)
	copy(h[8:], "WAVE")
	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], w.channels)
	binary.LittleEndian.PutUint32(h[24:], w.sampleRate)
	binary.LittleEndian.PutUint32(h[28:], w.sampleRate*uint32(w.channels)*bytesPerSample) // byte rate
	binary.LittleEndian.PutUint16(h[32:], w.channels*bytesPerSample)                      // block align
	binary.LittleEndian.PutUint16(h[34:], bitsPerSample)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], w.dataBytes)
	_, err := w.f.WriteAt(h[:], 0)
	return err
}

// WriteFloat32 appends interleaved float32 samples (range -1..1), converting
// each to little-endian int16 with clipping so values outside the range don't
// wrap around into noise.
func (w *Writer) WriteFloat32(samples []float32) error {
	out := make([]byte, len(samples)*bytesPerSample)
	for i, s := range samples {
		v := int32(s * 32767)
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(v)))
	}
	n, err := w.buf.Write(out)
	w.dataBytes += uint32(n)
	return err
}

// Close flushes buffered samples, patches the header sizes, and closes the file.
// Safe to call more than once.
func (w *Writer) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if err := w.buf.Flush(); err != nil {
		w.f.Close()
		return err
	}
	if err := w.writeHeader(); err != nil { // rewrite with real sizes
		w.f.Close()
		return err
	}
	return w.f.Close()
}

// Duration is a small convenience for logging: seconds of audio written so far.
func (w *Writer) Duration() float64 {
	frames := float64(w.dataBytes) / float64(uint32(w.channels)*bytesPerSample)
	if w.sampleRate == 0 {
		return 0
	}
	return frames / float64(w.sampleRate)
}
