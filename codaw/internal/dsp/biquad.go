package dsp

import "math"

// biquad is a second-order filter: five coefficients and two samples of memory, which between them
// make a low or high shelf, a peak, a low or high pass... The coefficients come from Robert
// Bristow-Johnson's Audio EQ Cookbook (https://www.w3.org/TR/audio-eq-cookbook/).
//
// The memory (x1, x2, y1, y2) is state, one set per channel. It survives coefficient changes, which is
// what lets a filter be retuned while it plays.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (f *biquad) process(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1 = f.x1, x
	f.y2, f.y1 = f.y1, y
	return y
}

// coefficients are a biquad's five numbers, normalised so a0 = 1.
type coefficients struct{ b0, b1, b2, a1, a2 float64 }

func (f *biquad) use(c coefficients) {
	f.b0, f.b1, f.b2, f.a1, f.a2 = c.b0, c.b1, c.b2, c.a1, c.a2
}

func normalise(b0, b1, b2, a0, a1, a2 float64) coefficients {
	return coefficients{b0 / a0, b1 / a0, b2 / a0, a1 / a0, a2 / a0}
}

// clampHz keeps a frequency inside what a sample rate can represent.
func clampHz(hz float64, rate int) float64 {
	return math.Min(math.Max(hz, 10), float64(rate)*0.45)
}

// lowShelf boosts or cuts everything below hz by db, with a shelf slope of 1 (as steep as it gets
// without a bump).
func lowShelf(hz, db float64, rate int) coefficients {
	a := math.Pow(10, db/40)
	w := 2 * math.Pi * clampHz(hz, rate) / float64(rate)
	cos, alpha := math.Cos(w), math.Sin(w)/2*math.Sqrt2
	sq := 2 * math.Sqrt(a) * alpha
	return normalise(
		a*((a+1)-(a-1)*cos+sq),
		2*a*((a-1)-(a+1)*cos),
		a*((a+1)-(a-1)*cos-sq),
		(a+1)+(a-1)*cos+sq,
		-2*((a-1)+(a+1)*cos),
		(a+1)+(a-1)*cos-sq,
	)
}

// highShelf boosts or cuts everything above hz by db.
func highShelf(hz, db float64, rate int) coefficients {
	a := math.Pow(10, db/40)
	w := 2 * math.Pi * clampHz(hz, rate) / float64(rate)
	cos, alpha := math.Cos(w), math.Sin(w)/2*math.Sqrt2
	sq := 2 * math.Sqrt(a) * alpha
	return normalise(
		a*((a+1)+(a-1)*cos+sq),
		-2*a*((a-1)+(a+1)*cos),
		a*((a+1)+(a-1)*cos-sq),
		(a+1)-(a-1)*cos+sq,
		2*((a-1)-(a+1)*cos),
		(a+1)-(a-1)*cos-sq,
	)
}

// peak boosts or cuts a band around hz by db, q wide.
func peak(hz, db, q float64, rate int) coefficients {
	a := math.Pow(10, db/40)
	w := 2 * math.Pi * clampHz(hz, rate) / float64(rate)
	cos, alpha := math.Cos(w), math.Sin(w)/(2*q)
	return normalise(
		1+alpha*a,
		-2*cos,
		1-alpha*a,
		1+alpha/a,
		-2*cos,
		1-alpha/a,
	)
}
