package mix

import "github.com/KevinGruber2001/codaw/internal/sample"

// clipState is a clip timed in frames, with its audio.
type clipState struct {
	buf        *sample.Buffer // shared and read-only: the cache's
	start, end uint64         // on the timeline, in frames
	offset     int            // where in the file it starts, in frames
	loop       bool
	gain       float32
}

// mixInto adds the part of the clip that falls in the block starting at pos into dst.
//
// The file is read from the offset to its end, then (when the clip loops) from its start to its end,
// again and again, until the clip ends. Without looping, the clip is silent once the file runs out.
func (c *clipState) mixInto(dst []float32, pos uint64, frames, fade int) {
	if c.end <= pos || c.start >= pos+uint64(frames) {
		return
	}
	data, ch := c.buf.Data, c.buf.Channels
	n := c.buf.Frames()
	if n == 0 {
		return
	}
	firstPass := max(0, n-c.offset) // frames of audio before the first loop

	f0 := int(max(c.start, pos) - pos)
	f1 := int(min(c.end, pos+uint64(frames)) - pos)
	for f := f0; f < f1; f++ {
		t := pos + uint64(f)
		elapsed := int(t - c.start)
		var i int
		switch {
		case elapsed < firstPass:
			i = c.offset + elapsed
		case c.loop:
			i = (elapsed - firstPass) % n
		default:
			return
		}

		g := c.gain
		if elapsed < fade {
			g *= float32(elapsed) / float32(fade)
		}
		if left := int(c.end - t); left < fade {
			g *= float32(left) / float32(fade)
		}
		l := data[i*ch]
		r := l
		if ch > 1 {
			r = data[i*ch+1]
		}
		dst[f*2] += l * g
		dst[f*2+1] += r * g
	}
}
