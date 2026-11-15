package dsp

import "math"

// Ceiling is the highest level the master limiter lets through: a little under full scale, so
// converting to integers when rendering never clips.
const Ceiling = 0.966 // -0.3 dBFS

// limiterRelease is how fast the limiter gives the level back after a peak.
const limiterRelease = 0.05 // seconds

// Limiter keeps the output under Ceiling, whatever comes in: the master's `limiter = true`.
//
// It reacts at once to a peak (the gain for that very sample is what brings it down to the ceiling)
// and lets go slowly. Reacting at once is what guarantees nothing gets through; letting go slowly is
// what stops it from distorting the waveform on every cycle.
type Limiter struct {
	gain    float64 // currently applied, <= 1
	release float64 // per-sample coefficient towards 1
}

// NewLimiter makes a limiter for a sample rate.
func NewLimiter(rate int) *Limiter {
	return &Limiter{gain: 1, release: math.Exp(-1 / (limiterRelease * float64(rate)))}
}

func (l *Limiter) Process(buf []float32, frames int) {
	for f := 0; f < frames; f++ {
		left, right := buf[f*2], buf[f*2+1]
		level := math.Max(math.Abs(float64(left)), math.Abs(float64(right)))
		l.gain = 1 + (l.gain-1)*l.release
		if level*l.gain > Ceiling {
			l.gain = Ceiling / level
		}
		g := float32(l.gain)
		buf[f*2], buf[f*2+1] = left*g, right*g
	}
}
