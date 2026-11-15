package schema

import "testing"

func TestTablesAreWellFormed(t *testing.T) {
	check := func(where string, keys []Key) {
		seen := map[string]bool{}
		for _, k := range keys {
			at := where + "." + k.Name
			if k.Name == "" || k.Doc == "" || k.Kind == "" {
				t.Errorf("%s: a key needs a name, a kind and a doc line", at)
			}
			if seen[k.Name] {
				t.Errorf("%s: listed twice", at)
			}
			seen[k.Name] = true
			if k.Kind != Number && (k.Min != nil || k.Max != nil || k.OneOf != nil || k.Unit != "") {
				t.Errorf("%s: only a number has a unit or a range", at)
			}
			if value, ok := AsNumber(k.Default); ok && k.Check(value) != "" {
				t.Errorf("%s: its default is not a valid value: %s", at, k.Check(value))
			}
		}
	}
	for _, file := range Files {
		for _, section := range file.Sections {
			check(string(file.Kind)+"/"+section.Name, section.Keys)
		}
	}
	for _, effect := range Effects {
		check("fx/"+effect.Name, effect.Params)
		for _, param := range effect.Params {
			if param.Kind != Number {
				t.Errorf("fx/%s.%s: effect parameters are numbers", effect.Name, param.Name)
			}
		}
	}
}

func TestCheck(t *testing.T) {
	track, _ := FileOf(TrackFile)
	top, _ := track.Section("")
	clip, _ := track.Section("clip")
	project, _ := FileOf(ProjectFile)
	transport, _ := project.Section("transport")
	key := func(s Section, name string) Key { k, _ := s.Key(name); return k }
	for _, tt := range []struct {
		key   Key
		value float64
		want  string
	}{
		{key(top, "gain"), -3, ""},
		{key(top, "gain"), 6, ""},
		{key(top, "gain"), -70, "must be between -60 and 6 dB, got -70"},
		{key(top, "pan"), 1.5, "must be between -1 and 1, got 1.5"},
		{key(clip, "start"), -1, "must be >= 0, got -1"},
		{key(clip, "end"), 1e9, ""},
		{key(transport, "sample_rate"), 48000, ""},
		{key(transport, "sample_rate"), 22050, "must be 44100, 48000, 88200 or 96000, got 22050"},
	} {
		if got := tt.key.Check(tt.value); got != tt.want {
			t.Errorf("%s = %v: got %q, want %q", tt.key.Name, tt.value, got, tt.want)
		}
	}
}

func TestResolve(t *testing.T) {
	reverb, _ := EffectNamed("reverb")
	got := reverb.Resolve(map[string]any{"wet": int64(1), "unknown": 3.0, "room_size": "big"})
	if len(got) != 2 || got["wet"] != 1 || got["room_size"] != 0.5 {
		t.Errorf("got %v, want the file's wet and the default room_size", got)
	}
}
