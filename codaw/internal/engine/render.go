package engine

import (
	"fmt"
	"log"

	"github.com/KevinGruber2001/codaw/internal/mix"
	"github.com/KevinGruber2001/codaw/internal/wav"
)

// renderBlock is how many frames Render mixes at a time: the same as live playback, so automation is
// sampled the same way in both.
const renderBlock = 512

// Render plays the whole project offline, faster than real time, into a 16-bit WAV at outPath: from
// beat 0 to the end of the last clip, plus tailSeconds for effects to ring out. Call Load first.
//
// It runs the same MixBlock as playback, so a render sounds like what you heard.
func (e *Engine) Render(outPath string, tailSeconds float64) error {
	p := e.store.Get()
	lastBeat := 0.0
	for _, t := range p.Tracks {
		for _, c := range t.Clips {
			lastBeat = max(lastBeat, c.End)
		}
	}
	if lastBeat == 0 {
		return fmt.Errorf("engine: nothing to render (no clips)")
	}
	rate := e.mixer.Rate()
	total := int(lastBeat*float64(rate)*60/p.Transport.BPM + tailSeconds*float64(rate))

	w, err := wav.Create(outPath, uint32(rate), mix.Channels)
	if err != nil {
		return err
	}
	defer w.Close()

	e.mixer.Seek(0)
	e.mixer.Play()
	buf := make([]float32, renderBlock*mix.Channels)
	for done := 0; done < total; done += renderBlock {
		n := min(renderBlock, total-done)
		e.mixer.MixBlock(buf[:n*mix.Channels])
		if err := w.WriteFloat32(buf[:n*mix.Channels]); err != nil {
			return err
		}
	}
	e.mixer.Stop()

	dur := w.Duration()
	if err := w.Close(); err != nil {
		return err
	}
	log.Printf("[render] wrote %s — %.2fs, %d Hz", outPath, dur, rate)
	return nil
}
