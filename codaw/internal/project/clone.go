package project

// Clone returns a deep copy of the project.
//
// Why this exists: the state Store uses copy-on-write. Every mutation runs on
// a fresh clone, which is then swapped in as the current project. Readers that
// grabbed the previous pointer (the audio engine's automation ticker, the
// watcher mid-diff) keep a fully consistent, never-again-modified snapshot —
// no data race, no locks held while reading.
//
// The copy must be deep: cloning just the Project struct would still share the
// Track/Bus/Master pointers and the FX param maps with the old snapshot, and
// mutating those would race with readers all the same. So we copy every level
// down to the maps.
func (p *Project) Clone() *Project {
	if p == nil {
		return nil
	}

	// Top-level struct: value copy gets Meta, Transport, Layout, RootDir.
	// (Layout contains string slices, but nothing ever mutates them after
	// load — they're treated as immutable, so sharing them is safe.)
	out := *p

	out.Master = cloneMaster(p.Master)

	out.Buses = make([]*Bus, len(p.Buses))
	for i, b := range p.Buses {
		out.Buses[i] = cloneBus(b)
	}

	out.Tracks = make([]*Track, len(p.Tracks))
	for i, t := range p.Tracks {
		out.Tracks[i] = cloneTrack(t)
	}

	return &out
}

func cloneMaster(m *Master) *Master {
	if m == nil {
		return nil
	}
	out := *m
	out.FX = cloneFXChain(m.FX)
	return &out
}

func cloneBus(b *Bus) *Bus {
	if b == nil {
		return nil
	}
	out := *b
	out.FX = cloneFXChain(b.FX)
	return &out
}

func cloneTrack(t *Track) *Track {
	if t == nil {
		return nil
	}
	out := *t
	out.FX = cloneFXChain(t.FX)

	// Clips are value structs — copying the slice copies the elements.
	out.Clips = append([]Clip(nil), t.Clips...)

	// Automation lanes hold a points slice each; copy those too.
	out.Automation = make([]Automation, len(t.Automation))
	for i, a := range t.Automation {
		out.Automation[i] = a
		out.Automation[i].Points = append([]AutomationPoint(nil), a.Points...)
	}
	return &out
}

func cloneFXChain(fxs []FX) []FX {
	if fxs == nil {
		return nil
	}
	out := make([]FX, len(fxs))
	for i, f := range fxs {
		out[i] = f
		// The params map is the mutable part (SetFXParam writes into it) —
		// it must not be shared between snapshots.
		out[i].Params = make(map[string]any, len(f.Params))
		for k, v := range f.Params {
			out[i].Params[k] = v
		}
	}
	return out
}
