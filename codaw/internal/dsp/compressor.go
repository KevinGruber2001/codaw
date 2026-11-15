package dsp

import "math"

func init() { Register("compressor", newCompressor) }

// compressor turns down whatever is louder than its threshold: by ratio, so with 4:1 a peak 8 dB over
// comes out 2 dB over. It listens to both channels together and turns both down by the same amount,
// so the stereo image does not lean when one side gets louder.
//
// The gain reduction moves at two speeds: attack_ms when it has to turn down more, release_ms when it
// may turn down less. That envelope is the compressor's state, in dB.
type compressor struct {
	rate                                        int
	threshold, ratio, attack, release, makeupDB smoothed
	reduction                                   float64 // dB currently taken off, >= 0
}

func newCompressor(rate int) Effect { return &compressor{rate: rate} }

func (c *compressor) SetParam(name string, v float64) bool {
	switch name {
	case "threshold":
		c.threshold.set(v)
	case "ratio":
		c.ratio.set(v)
	case "attack_ms":
		c.attack.set(v)
	case "release_ms":
		c.release.set(v)
	case "makeup_db":
		c.makeupDB.set(v)
	default:
		return false
	}
	return true
}

func (c *compressor) snap() {
	c.threshold.snap()
	c.ratio.snap()
	c.attack.snap()
	c.release.snap()
	c.makeupDB.snap()
}

// timeCoefficient is how much of the old value a one-pole envelope keeps per sample, for a time
// constant of ms milliseconds.
func timeCoefficient(ms float64, rate int) float64 {
	if ms <= 0 {
		return 0
	}
	return math.Exp(-1 / (ms / 1000 * float64(rate)))
}

func (c *compressor) Process(buf []float32, frames int) {
	threshold := c.threshold.next(frames, c.rate)
	ratio := math.Max(1, c.ratio.next(frames, c.rate))
	attack := timeCoefficient(c.attack.next(frames, c.rate), c.rate)
	release := timeCoefficient(c.release.next(frames, c.rate), c.rate)
	makeup := c.makeupDB.next(frames, c.rate)
	slope := 1 - 1/ratio

	for f := 0; f < frames; f++ {
		l, r := buf[f*2], buf[f*2+1]
		level := math.Max(math.Abs(float64(l)), math.Abs(float64(r)))
		want := 0.0
		if level > 1e-9 {
			if over := Lin2DB(level) - threshold; over > 0 {
				want = over * slope
			}
		}
		k := release
		if want > c.reduction {
			k = attack
		}
		c.reduction = want + (c.reduction-want)*k
		g := float32(DB2Lin(makeup - c.reduction))
		buf[f*2], buf[f*2+1] = l*g, r*g
	}
}
