package dsp

func init() { Register("eq_3band", newEQ) }

// eq is the three-band equalizer: a low shelf, a peak and a high shelf in series, per channel.
type eq struct {
	rate                   int
	lowHz, lowDB           smoothed
	midHz, midDB           smoothed
	highHz, highDB         smoothed
	filters                [2][3]biquad // channel, band
	tuned                  bool
	lastLH, lastLD, lastMH float64
	lastMD, lastHH, lastHD float64
}

// The width of the middle band. Fixed: the schema has no q, and about an octave and a half is what a
// "mid" knob on a mixing desk does.
const eqMidQ = 0.7

func newEQ(rate int) Effect { return &eq{rate: rate} }

func (e *eq) SetParam(name string, v float64) bool {
	switch name {
	case "low_hz":
		e.lowHz.set(v)
	case "low_db":
		e.lowDB.set(v)
	case "mid_hz":
		e.midHz.set(v)
	case "mid_db":
		e.midDB.set(v)
	case "high_hz":
		e.highHz.set(v)
	case "high_db":
		e.highDB.set(v)
	default:
		return false
	}
	return true
}

func (e *eq) snap() {
	for _, s := range []*smoothed{&e.lowHz, &e.lowDB, &e.midHz, &e.midDB, &e.highHz, &e.highDB} {
		s.snap()
	}
}

func (e *eq) Process(buf []float32, frames int) {
	lh, ld := e.lowHz.next(frames, e.rate), e.lowDB.next(frames, e.rate)
	mh, md := e.midHz.next(frames, e.rate), e.midDB.next(frames, e.rate)
	hh, hd := e.highHz.next(frames, e.rate), e.highDB.next(frames, e.rate)
	// Retune only when something moved: computing coefficients takes a few trigonometric functions.
	if !e.tuned || lh != e.lastLH || ld != e.lastLD || mh != e.lastMH || md != e.lastMD || hh != e.lastHH || hd != e.lastHD {
		low, mid, high := lowShelf(lh, ld, e.rate), peak(mh, md, eqMidQ, e.rate), highShelf(hh, hd, e.rate)
		for ch := range e.filters {
			e.filters[ch][0].use(low)
			e.filters[ch][1].use(mid)
			e.filters[ch][2].use(high)
		}
		e.lastLH, e.lastLD, e.lastMH, e.lastMD, e.lastHH, e.lastHD = lh, ld, mh, md, hh, hd
		e.tuned = true
	}
	for f := 0; f < frames; f++ {
		for ch := 0; ch < 2; ch++ {
			x := float64(buf[f*2+ch])
			fs := &e.filters[ch]
			x = fs[0].process(x)
			x = fs[1].process(x)
			x = fs[2].process(x)
			buf[f*2+ch] = float32(x)
		}
	}
}
