package dsp

func init() { Register("reverb", newReverb) }

// reverb is Jezar's Freeverb: eight comb filters in parallel, then four all-pass filters in series,
// per channel. The combs are the room's echoes, each a delay line feeding back into itself through
// a low-pass (walls absorb highs); the all-passes smear those echoes into a wash. The right channel's
// delay lines are a little longer than the left's, which is what makes it sound wide.
//
// room_size is how long the echoes keep feeding back; wet how much of the reverb is mixed in.
type reverb struct {
	rate      int
	roomSize  smoothed
	wet       smoothed
	combs     [2][8]comb
	allpasses [2][4]allpass
}

// Freeverb's delay lengths, in samples at 44.1 kHz, and the extra length of the right channel's.
var (
	combTuning    = [8]int{1116, 1188, 1277, 1356, 1422, 1491, 1557, 1617}
	allpassTuning = [4]int{556, 441, 341, 225}
)

const (
	stereoSpread = 23
	// The input is turned down a lot before the combs, which add up eight feedback loops, and the
	// result turned up again after.
	reverbInputGain = 0.015
	reverbWetGain   = 3.0
	reverbDamping   = 0.2 // how much highs each echo loses
)

func newReverb(rate int) Effect {
	r := &reverb{rate: rate}
	scale := float64(rate) / 44100
	for ch := 0; ch < 2; ch++ {
		spread := ch * stereoSpread
		for i, n := range combTuning {
			r.combs[ch][i].buf = make([]float64, int(float64(n+spread)*scale))
		}
		for i, n := range allpassTuning {
			r.allpasses[ch][i].buf = make([]float64, int(float64(n+spread)*scale))
		}
	}
	return r
}

func (r *reverb) SetParam(name string, v float64) bool {
	switch name {
	case "room_size":
		r.roomSize.set(v)
	case "wet":
		r.wet.set(v)
	default:
		return false
	}
	return true
}

func (r *reverb) snap() {
	r.roomSize.snap()
	r.wet.snap()
}

func (r *reverb) Process(buf []float32, frames int) {
	feedback := 0.7 + 0.28*r.roomSize.next(frames, r.rate)
	wet := r.wet.next(frames, r.rate)
	dry := 1 - wet
	for f := 0; f < frames; f++ {
		in := (float64(buf[f*2]) + float64(buf[f*2+1])) * reverbInputGain
		for ch := 0; ch < 2; ch++ {
			out := 0.0
			for i := range r.combs[ch] {
				out += r.combs[ch][i].process(in, feedback)
			}
			for i := range r.allpasses[ch] {
				out = r.allpasses[ch][i].process(out)
			}
			x := float64(buf[f*2+ch])
			buf[f*2+ch] = float32(x*dry + out*wet*reverbWetGain)
		}
	}
}

// comb is a delay line that feeds back into itself, through a one-pole low-pass.
type comb struct {
	buf    []float64
	i      int
	lowest float64 // the low-pass's memory
}

func (c *comb) process(in, feedback float64) float64 {
	out := c.buf[c.i]
	c.lowest = out*(1-reverbDamping) + c.lowest*reverbDamping
	c.buf[c.i] = in + c.lowest*feedback
	if c.i++; c.i == len(c.buf) {
		c.i = 0
	}
	return out
}

// allpass passes every frequency at the same level but delays each by a different amount, which
// is what diffuses the combs' distinct echoes.
type allpass struct {
	buf []float64
	i   int
}

func (a *allpass) process(in float64) float64 {
	delayed := a.buf[a.i]
	a.buf[a.i] = in + delayed*0.5
	if a.i++; a.i == len(a.buf) {
		a.i = 0
	}
	return delayed - in
}
