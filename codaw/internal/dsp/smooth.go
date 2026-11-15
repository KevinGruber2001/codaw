package dsp

import "math"

// smoothTime is how long a parameter takes to get most of the way to a new value. Long enough that a
// jump is not a click, short enough that a turned knob feels immediate.
const smoothTime = 0.02 // seconds

// smoothed is a parameter that glides to its target instead of jumping to it. It moves once per
// block, by the part of the way that a block's worth of time covers: a one-pole filter, sampled at
// the block rate.
type smoothed struct {
	value, target float64
}

func (s *smoothed) set(v float64) { s.target = v }

// snap jumps to the target: for a new effect, whose first values are not a change.
func (s *smoothed) snap() { s.value = s.target }

// next moves on by one block of frames and returns the new value.
func (s *smoothed) next(frames, rate int) float64 {
	if s.value == s.target {
		return s.value
	}
	k := 1 - math.Exp(-float64(frames)/(smoothTime*float64(rate)))
	s.value += (s.target - s.value) * k
	if math.Abs(s.target-s.value) < 1e-6*math.Max(1, math.Abs(s.target)) {
		s.value = s.target
	}
	return s.value
}

// DB2Lin converts decibels to a linear factor: 0 dB is 1, -6 dB about 0.5.
func DB2Lin(db float64) float64 { return math.Pow(10, db/20) }

// Lin2DB converts a linear level to decibels; silence is -inf.
func Lin2DB(lin float64) float64 { return 20 * math.Log10(lin) }
