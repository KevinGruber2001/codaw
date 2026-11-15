package mix

import "github.com/KevinGruber2001/codaw/internal/sample"

type clipState struct {
    buf *sample.Buffer // shared, read-only, from the Cache

    startFrame   uint64 // Clip.Start beats -> frames
    endFrame     uint64 // Clip.End   beats -> frames
    offsetFrames int    // Clip.Offset seconds -> frames
    loop         bool
    gain         float32 // db2lin(Clip.Gain)
}
