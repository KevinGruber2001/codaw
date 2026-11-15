package lsp

import (
	"sort"
	"strings"

	"github.com/KevinGruber2001/codaw/internal/schema"
)

// What CodaW's files may contain comes from internal/schema. This file only puts it in the shape
// completion, hover and the checks ask for.

type fileKind int

const (
	kindUnknown fileKind = iota
	kindProject
	kindTrack
	kindBus
	kindMaster
)

var schemaKinds = map[schema.FileKind]fileKind{
	schema.ProjectFile: kindProject,
	schema.TrackFile:   kindTrack,
	schema.BusFile:     kindBus,
	schema.MasterFile:  kindMaster,
}

// key is one key a section may have, as shown to the user.
type key struct {
	name   string
	kind   string   // "number", "string", "bool" or "array"
	doc    string   // one line
	detail string   // unit, range and default: "dB, -60 to 6"
	values []string // what to suggest after "key = ", if the value is one of a few
}

type effect struct {
	doc    string
	params []key
}

var (
	// sections lists, per kind of file, the keys of each section; "" is the top of a file.
	sections = map[fileKind]map[string][]key{}
	// headers are the sections a kind of file may have, as typed: "[transport]", "[[clip]]".
	headers     = map[fileKind][]string{}
	sectionDocs = map[string]string{}
	effects     = map[string]effect{}
	fxTypeKey   key
)

func init() {
	for _, file := range schema.Files {
		kind := schemaKinds[file.Kind]
		sections[kind] = map[string][]key{}
		for _, section := range file.Sections {
			sections[kind][section.Name] = toKeys(section.Keys)
			if section.Name == "" {
				continue
			}
			header := "[" + section.Name + "]"
			if section.List {
				header = "[" + header + "]"
			}
			headers[kind] = append(headers[kind], header)
			sectionDocs[section.Name] = section.Doc
		}
	}
	for _, e := range schema.Effects {
		effects[e.Name] = effect{doc: e.Doc, params: toKeys(e.Params)}
	}
	fxTypeKey, _ = findKey(sections[kindTrack]["fx"], "type")
	fxTypeKey.doc += " One of `" + strings.Join(effectNames(), "`, `") + "`."
}

func toKeys(keys []schema.Key) []key {
	out := make([]key, len(keys))
	for i, k := range keys {
		out[i] = key{name: k.Name, kind: string(k.Kind), doc: k.Doc, detail: detail(k)}
		switch {
		case k.Kind == schema.List:
			out[i].kind = "array"
		case k.Kind == schema.Bool:
			out[i].values = []string{"true", "false"}
		}
		for _, value := range k.OneOf {
			out[i].values = append(out[i].values, schema.Format(value))
		}
		for _, value := range k.Suggest {
			out[i].values = append(out[i].values, `"`+value+`"`)
		}
	}
	return out
}

// detail is a key's unit, range and default in a few words: "dB, -60 to 6", "Hz, 20 to 20000,
// default 100", "seconds, from 0". A default of zero goes without saying.
func detail(k schema.Key) string {
	var parts []string
	if k.Unit != "" {
		parts = append(parts, k.Unit)
	}
	switch {
	case k.Min != nil && k.Max != nil:
		parts = append(parts, schema.Format(*k.Min)+" to "+schema.Format(*k.Max))
	case k.Min != nil:
		parts = append(parts, "from "+schema.Format(*k.Min))
	case k.Max != nil:
		parts = append(parts, "up to "+schema.Format(*k.Max))
	}
	if value, ok := schema.AsNumber(k.Default); ok && value != 0 {
		parts = append(parts, "default "+schema.Format(value))
	}
	return strings.Join(parts, ", ")
}

// keysFor are the keys of a section in a kind of file; fxType is the type of the [[fx]] the cursor
// is in, for its parameters.
func keysFor(kind fileKind, section, fxType string) []key {
	if section == "fx" {
		keys := []key{fxTypeKey}
		if effect, ok := effects[fxType]; ok {
			keys = append(keys, effect.params...)
		}
		return keys
	}
	return sections[kind][section]
}

// keysOf are the keys a table of a document may have.
func keysOf(kind fileKind, t *table) []key {
	return keysFor(kind, t.name, t.text("type"))
}

func findKey(keys []key, name string) (key, bool) {
	for _, k := range keys {
		if k.name == name {
			return k, true
		}
	}
	return key{}, false
}

// sectionOf finds a section among those a kind of file may have, and whether it is a list ([[x]]).
func sectionOf(kind fileKind, name string) (array, ok bool) {
	for _, header := range headers[kind] {
		if strings.Trim(header, "[]") == name {
			return strings.HasPrefix(header, "[["), true
		}
	}
	return false, false
}

func effectNames() []string {
	names := make([]string, 0, len(effects))
	for name := range effects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// closest is the candidate a misspelled name most likely meant, or "".
func closest(name string, candidates []string) string {
	best, bestDistance := "", 3
	for _, candidate := range candidates {
		if d := distance(name, candidate); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	return best
}

func distance(a, b string) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		previous := row[0]
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := previous
			if a[i-1] != b[j-1] {
				cost = min(previous, row[j], row[j-1]) + 1
			}
			previous, row[j] = row[j], cost
		}
	}
	return row[len(b)]
}
