package engine

import (
	"log"

	"github.com/KevinGruber2001/codaw/internal/audio"
	"github.com/KevinGruber2001/codaw/internal/project"
)

// buildChain turns a project FX list into audio effects for one owner.
//
// The returned slice is index-aligned with fxs: an effect type we don't
// implement yet (compressor) becomes a nil entry rather than being dropped, so
// an FXParamPayload's FXIndex always maps to the right slot. attachable() later
// strips the nils for graph wiring.
func (e *Engine) buildChain(fxs []project.FX) ([]*audio.Effect, error) {
	chain := make([]*audio.Effect, len(fxs))
	for i, f := range fxs {
		ef, err := e.buildEffect(f)
		if err != nil {
			return nil, err
		}
		chain[i] = ef // may be nil for unimplemented types
	}
	return chain, nil
}

// buildEffect constructs a single effect from its TOML definition. Returns
// (nil, nil) for types that are valid project data but have no DSP yet, so the
// caller can keep the slot aligned and carry on.
func (e *Engine) buildEffect(f project.FX) (*audio.Effect, error) {
	switch f.Type {
	case "eq_3band":
		return e.audio.NewEQ3Band(
			fxFloat(f, "low_hz", 100), fxFloat(f, "low_db", 0),
			fxFloat(f, "mid_hz", 1000), fxFloat(f, "mid_db", 0),
			fxFloat(f, "high_hz", 8000), fxFloat(f, "high_db", 0),
		)
	case "reverb":
		return e.audio.NewReverb(fxFloat(f, "room_size", 0.5), fxFloat(f, "wet", 0.3))
	case "compressor":
		log.Printf("[engine] fx %q has no DSP yet — passing audio through unprocessed", f.Type)
		return nil, nil
	default:
		log.Printf("[engine] unknown fx %q — skipping", f.Type)
		return nil, nil
	}
}

// attachable returns the chain with nil (unimplemented) entries removed, ready
// to be spliced into the node graph in order.
func attachable(chain []*audio.Effect) []*audio.Effect {
	out := make([]*audio.Effect, 0, len(chain))
	for _, ef := range chain {
		if ef != nil {
			out = append(out, ef)
		}
	}
	return out
}

// fxFloat reads a numeric FX parameter, coercing whatever TOML decoded it as
// (int64 or float64) to float64. Falls back to def when missing or unparseable.
func fxFloat(f project.FX, key string, def float64) float64 {
	if v, ok := f.Params[key]; ok {
		if x, ok := toFloat64(v); ok {
			return x
		}
	}
	return def
}

// toFloat64 coerces the dynamically-typed values TOML produces into float64.
// TOML numbers arrive as int64 (e.g. 100) or float64 (e.g. 1.5).
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
