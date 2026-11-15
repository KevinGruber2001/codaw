package mix

import "github.com/KevinGruber2001/codaw/internal/project"

type Mixer struct {
	proj  *project.Project
	rate  int
	chans int // always 2 for now (stereo)

	oos   uint64      // playhead position
	clips []clipState

	trackBuf  []float32
	busBufs   map[string][]float32
	masterBuf []float32
}

func (m *Mixer) MixBlock(out []float32) {

}
