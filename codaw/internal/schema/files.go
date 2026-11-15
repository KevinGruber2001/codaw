package schema

func f(value float64) *float64 { return &value }

func gain(doc string) Key {
	return Key{Name: "gain", Kind: Number, Unit: "dB", Min: f(-60), Max: f(6), Default: 0.0, Doc: doc}
}

var fxSection = Section{
	Name: "fx", List: true,
	Doc: "An effect. Effects run in the order they are written; automation refers to them by position, as `fx[0]`, `fx[1]`, …",
	Keys: []Key{
		{Name: "type", Kind: String, Required: true, Doc: "Which effect."},
	},
}

// Files are the kinds of file a project is made of.
var Files = []File{
	{Kind: ProjectFile, Sections: []Section{
		{Name: "project", Doc: "What the project is called and who made it.", Keys: []Key{
			{Name: "name", Kind: String, Doc: "The project's name."},
			{Name: "version", Kind: String, Doc: "A version of your choosing."},
			{Name: "author", Kind: String, Doc: "Who made it."},
		}},
		{Name: "transport", Doc: "Tempo and audio format.", Keys: []Key{
			{Name: "bpm", Kind: Number, Min: f(20), Max: f(300), Required: true, Doc: "Tempo, in beats per minute."},
			{Name: "time_sig", Kind: String, Required: true, Suggest: []string{"4/4", "3/4", "6/8"}, Doc: "Time signature: beats per bar / beat unit."},
			{Name: "sample_rate", Kind: Number, Unit: "Hz", OneOf: []float64{44100, 48000, 88200, 96000}, Required: true, Doc: "Sample rate the project renders at."},
			{Name: "bit_depth", Kind: Number, OneOf: []float64{16, 24, 32}, Required: true, Doc: "Bit depth of rendered files."},
		}},
		{Name: "layout", Doc: "The files that make up the project: the master, the buses, the tracks.", Keys: []Key{
			{Name: "master", Kind: String, Doc: "The master file, relative to project.toml."},
			{Name: "buses", Kind: List, Doc: "The bus files, in order."},
			{Name: "tracks", Kind: List, Doc: "The track files, in the order the views show them."},
		}},
	}},
	{Kind: TrackFile, Sections: []Section{
		{Name: "", Keys: []Key{
			{Name: "id", Kind: String, Required: true, Doc: "The track's name; unique among the tracks."},
			{Name: "bus", Kind: String, Doc: "The bus this track feeds. Without one, it goes straight to the master."},
			gain("The track's level."),
			{Name: "pan", Kind: Number, Min: f(-1), Max: f(1), Default: 0.0, Doc: "Position in the stereo field: -1 is left, 1 is right."},
			{Name: "mute", Kind: Bool, Default: false, Doc: "Silences the track."},
			{Name: "solo", Kind: Bool, Default: false, Doc: "Plays only the soloed tracks."},
		}},
		{Name: "clip", List: true,
			Doc: "A piece of audio on the timeline: a file, and where it starts and ends in beats. To split a clip, write two that share a file.",
			Keys: []Key{
				{Name: "file", Kind: String, Required: true, Doc: "The audio file, relative to project.toml."},
				{Name: "start", Kind: Number, Unit: "beats", Min: f(0), Default: 0.0, Doc: "Where the clip starts."},
				{Name: "end", Kind: Number, Unit: "beats", Required: true, Doc: "Where the clip ends; after start."},
				{Name: "offset", Kind: Number, Unit: "seconds", Min: f(0), Default: 0.0, Doc: "Where in the audio file the clip begins."},
				{Name: "loop", Kind: Bool, Default: false, Doc: "Repeats the audio until the clip ends: from the offset once, then from the file's start."},
				gain("The clip's level, on top of the track's."),
			}},
		fxSection,
		{Name: "automation", List: true, Doc: "A lane that changes one value over time, between its `[[automation.point]]`s.", Keys: []Key{
			{Name: "target", Kind: String, Required: true,
				Doc: "What the lane changes: `gain`, `pan`, or a parameter of one of the track's effects as `fx[N].param`, N counting the `[[fx]]` from 0."},
		}},
		{Name: "automation.point", List: true, Doc: "A point of the `[[automation]]` above: a beat and the value there.", Keys: []Key{
			{Name: "beat", Kind: Number, Unit: "beats", Min: f(0), Default: 0.0, Doc: "Where the point is."},
			{Name: "value", Kind: Number, Default: 0.0, Doc: "The target's value there; in between, it changes in a straight line."},
		}},
	}},
	{Kind: BusFile, Sections: []Section{
		{Name: "", Keys: []Key{
			{Name: "id", Kind: String, Required: true, Doc: "The bus's name, which tracks send to with bus = \"…\"."},
			gain("The bus's level."),
		}},
		fxSection,
	}},
	{Kind: MasterFile, Sections: []Section{
		{Name: "", Keys: []Key{
			gain("The master's level."),
			{Name: "limiter", Kind: Bool, Default: false, Doc: "A limiter at the end of the master, after its effects: nothing louder than -0.3 dBFS gets through."},
		}},
		fxSection,
	}},
}

func hz(name string, def float64, doc string) Key {
	return Key{Name: name, Kind: Number, Unit: "Hz", Min: f(20), Max: f(20000), Default: def, Doc: doc}
}

func db(name string, doc string) Key {
	return Key{Name: name, Kind: Number, Unit: "dB", Min: f(-24), Max: f(24), Default: 0.0, Doc: doc}
}

func unit(name string, def float64, doc string) Key {
	return Key{Name: name, Kind: Number, Min: f(0), Max: f(1), Default: def, Doc: doc}
}

// Effects are the effect types the engine knows; internal/dsp implements them.
var Effects = []Effect{
	{
		Name: "eq_3band",
		Doc:  "A three-band equalizer: low shelf, peak, high shelf.",
		Params: []Key{
			hz("low_hz", 100, "Low shelf frequency."),
			db("low_db", "Low shelf boost or cut."),
			hz("mid_hz", 1000, "Peak frequency."),
			db("mid_db", "Peak boost or cut."),
			hz("high_hz", 8000, "High shelf frequency."),
			db("high_db", "High shelf boost or cut."),
		},
	},
	{
		Name: "reverb",
		Doc:  "A room reverb (Freeverb).",
		Params: []Key{
			unit("room_size", 0.5, "Size of the room: how long the reverb rings."),
			unit("wet", 0.3, "How much of the effect is heard."),
		},
	},
	{
		Name: "compressor",
		Doc:  "A compressor: turns down what is louder than the threshold, both channels together.",
		Params: []Key{
			{Name: "threshold", Kind: Number, Unit: "dB", Min: f(-60), Max: f(0), Default: -18.0, Doc: "Level above which it compresses."},
			{Name: "ratio", Kind: Number, Min: f(1), Max: f(20), Default: 4.0, Doc: "How strongly: 4 means 4 dB over the threshold come out as 1."},
			{Name: "attack_ms", Kind: Number, Unit: "ms", Min: f(0), Max: f(500), Default: 10.0, Doc: "How fast it reacts."},
			{Name: "release_ms", Kind: Number, Unit: "ms", Min: f(0), Max: f(5000), Default: 100.0, Doc: "How fast it lets go."},
			{Name: "makeup_db", Kind: Number, Unit: "dB", Min: f(-24), Max: f(24), Default: 0.0, Doc: "Level added after compressing."},
		},
	},
}
