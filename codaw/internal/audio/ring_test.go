package audio

import (
	"sync"
	"testing"
)

func TestRingWrapsAround(t *testing.T) {
	rb := newRing(8)
	out := make([]float32, 3)
	for round := 0; round < 10; round++ {
		in := []float32{float32(round), float32(round) + 0.25, float32(round) + 0.5}
		rb.write(in)
		if rb.read(out) {
			t.Fatalf("round %d: underrun with 3 written", round)
		}
		for i := range in {
			if out[i] != in[i] {
				t.Fatalf("round %d: read %v, wrote %v", round, out, in)
			}
		}
	}
}

func TestRingUnderrunIsSilence(t *testing.T) {
	rb := newRing(8)
	rb.write([]float32{1, 2})
	out := []float32{9, 9, 9, 9}
	if !rb.read(out) {
		t.Error("no underrun reported")
	}
	if want := []float32{1, 2, 0, 0}; out[0] != want[0] || out[1] != want[1] || out[2] != 0 || out[3] != 0 {
		t.Errorf("read %v, want %v", out, want)
	}
}

// One writer and one reader at the same time see every sample, in order (run with -race).
func TestRingConcurrent(t *testing.T) {
	rb := newRing(64)
	const total = 100000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		next := float32(0)
		block := make([]float32, 7)
		for next < total {
			if rb.space() < len(block) {
				continue
			}
			for i := range block {
				block[i] = next
				next++
			}
			rb.write(block)
		}
	}()
	want := float32(0)
	out := make([]float32, 5)
	for want < total {
		n := min(len(out), rb.buffered())
		if n == 0 {
			continue
		}
		rb.read(out[:n])
		for _, x := range out[:n] {
			if x != want {
				t.Fatalf("read %v, want %v", x, want)
			}
			want++
		}
	}
	wg.Wait()
}
