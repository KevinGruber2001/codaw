package audio

import "sync/atomic"

// ring is a circular buffer of float32 with exactly one writer (the mixer's goroutine) and one reader
// (the device callback).
//
// With one of each, two counters are all the synchronisation needed: the writer only ever moves w,
// the reader only ever moves r, and each reads the other's with an atomic load. No mutex, which
// matters because the reader is the device thread and must never wait.
//
// The counters only grow; a position in buf is the counter modulo its length. w - r is how much is
// waiting to be played.
type ring struct {
	buf  []float32
	w, r atomic.Uint64
}

func newRing(samples int) *ring { return &ring{buf: make([]float32, samples)} }

// buffered is how many samples are waiting to be read.
func (rb *ring) buffered() int { return int(rb.w.Load() - rb.r.Load()) }

// space is how many samples can be written without overwriting unread ones.
func (rb *ring) space() int { return len(rb.buf) - rb.buffered() }

// write appends src. The caller has checked there is space for it.
func (rb *ring) write(src []float32) {
	w := rb.w.Load()
	at := int(w % uint64(len(rb.buf)))
	n := copy(rb.buf[at:], src)
	copy(rb.buf, src[n:]) // what did not fit before the end wraps round to the start
	rb.w.Store(w + uint64(len(src)))
}

// read fills dst with what is waiting. If not enough is (an underrun), the rest is silence and read
// says so: the device must always get its samples, and a moment of silence is a far better failure
// than a stall.
func (rb *ring) read(dst []float32) (underrun bool) {
	r := rb.r.Load()
	n := min(len(dst), int(rb.w.Load()-r))
	at := int(r % uint64(len(rb.buf)))
	m := copy(dst[:n], rb.buf[at:])
	copy(dst[m:n], rb.buf)
	clear(dst[n:])
	rb.r.Store(r + uint64(n))
	return n < len(dst)
}
