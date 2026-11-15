package project

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/KevinGruber2001/codaw/internal/schema"
)

// The structs the TOML decodes into and the schema describe the same files. A field without a
// key would be unknown to the editor and the language server; a key without a field would be
// suggested and then ignored by the engine.
func TestStructsMatchTheSchema(t *testing.T) {
	for _, tt := range []struct {
		kind    schema.FileKind
		section string
		value   any
	}{
		{schema.ProjectFile, "project", ProjectMeta{}},
		{schema.ProjectFile, "transport", Transport{}},
		{schema.ProjectFile, "layout", Layout{}},
		{schema.TrackFile, "", Track{}},
		{schema.TrackFile, "clip", Clip{}},
		{schema.TrackFile, "automation", Automation{}},
		{schema.TrackFile, "automation.point", AutomationPoint{}},
		{schema.BusFile, "", Bus{}},
		{schema.MasterFile, "", Master{}},
	} {
		file, _ := schema.FileOf(tt.kind)
		section, ok := file.Section(tt.section)
		if !ok {
			t.Errorf("%s: the schema has no section %q", tt.kind, tt.section)
			continue
		}
		fields := map[string]schema.Kind{}
		typ := reflect.TypeOf(tt.value)
		for i := 0; i < typ.NumField(); i++ {
			tag := typ.Field(i).Tag.Get("toml")
			kind, isKey := kindOf(typ.Field(i).Type)
			if tag == "" || tag == "-" || !isKey {
				continue // a nested section ([[clip]], [[fx]]) or not in the file at all
			}
			fields[tag] = kind
		}
		var problems []string
		for _, key := range section.Keys {
			kind, ok := fields[key.Name]
			switch {
			case !ok:
				problems = append(problems, "key "+key.Name+" has no field")
			case kind != key.Kind:
				problems = append(problems, "key "+key.Name+" is a "+string(key.Kind)+", its field a "+string(kind))
			}
			delete(fields, key.Name)
		}
		for name := range fields {
			problems = append(problems, "field "+name+" has no key")
		}
		sort.Strings(problems)
		if len(problems) > 0 {
			t.Errorf("%s [%s] and %s disagree: %s", tt.kind, tt.section, typ.Name(), strings.Join(problems, "; "))
		}
	}
}

func kindOf(t reflect.Type) (schema.Kind, bool) {
	switch t.Kind() {
	case reflect.Float64, reflect.Int:
		return schema.Number, true
	case reflect.String:
		return schema.String, true
	case reflect.Bool:
		return schema.Bool, true
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			return schema.List, true
		}
	}
	return "", false
}

func TestEffectParametersAreValidated(t *testing.T) {
	p := &Project{
		Transport: Transport{BPM: 120, TimeSig: "4/4", SampleRate: 48000, BitDepth: 24},
		Master: &Master{SourceFile: "/p/master.toml", FX: []FX{
			{Type: "reverb", Params: map[string]any{"wet": 1.5, "room_size": 0.2}},
			{Type: "eq_3band", Params: map[string]any{"low_hz": int64(5)}},
			{Type: "made_up", Params: map[string]any{"wet": 99.0}},
		}},
	}
	err := Validate(p)
	if err == nil {
		t.Fatal("want two problems, got none")
	}
	got := err.Error()
	for _, want := range []string{"fx[0].wet — must be between 0 and 1, got 1.5", "fx[1].low_hz — must be between 20 and 20000 Hz, got 5"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if len(err.(ValidationErrors)) != 2 {
		t.Errorf("want exactly two problems:\n%s", got)
	}
}
