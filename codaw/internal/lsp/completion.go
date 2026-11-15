package lsp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// candidate is one suggestion for a value: its text (without quotes), whether it is a string (and so
// needs quotes), and what to show next to it.
type candidate struct {
	text   string
	quoted bool
	detail string
	doc    string
}

// completion suggests what fits at the cursor: section headers on a "[" line, the keys a section may
// still have, and values: the fixed ones from the schema, and the project's own (its buses, samples,
// effect parameters, files).
func (s *state) completion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	text := s.text(path)
	c := cursorAt(text, int(params.Position.Line), int(params.Position.Character))
	kind, p := projectOf(path)
	if kind == kindUnknown {
		return nil, nil
	}
	replace := protocol.Range{
		Start: protocol.Position{Line: params.Position.Line, Character: protocol.UInteger(c.wordStart)},
		End:   protocol.Position{Line: params.Position.Line, Character: protocol.UInteger(max(c.wordEnd, int(params.Position.Character)))},
	}

	var items []protocol.CompletionItem
	switch {
	case c.onHeader:
		for i, header := range headers[kind] {
			items = append(items, item(i, header, header, protocol.CompletionItemKindModule, "", "", replace))
		}
	case c.onKey:
		for i, k := range keysFor(kind, c.section, c.fxType) {
			if c.present[k.name] {
				continue // already set in this section
			}
			items = append(items, item(i, k.name, k.name+" = ", protocol.CompletionItemKindProperty, k.detail, k.doc, replace))
		}
	case c.onValue:
		for i, v := range valuesFor(c, kind, p, path) {
			insert := v.text
			if v.quoted && !c.inString {
				insert = `"` + v.text + `"`
			}
			items = append(items, item(i, v.text, insert, protocol.CompletionItemKindValue, v.detail, v.doc, replace))
		}
	}
	return items, nil
}

func item(order int, label, insert string, kind protocol.CompletionItemKind, detail, doc string, replace protocol.Range) protocol.CompletionItem {
	sortText := fmt.Sprintf("%03d", order) // in the schema's order, not alphabetical
	it := protocol.CompletionItem{
		Label:    label,
		Kind:     &kind,
		SortText: &sortText,
		TextEdit: protocol.TextEdit{Range: replace, NewText: insert},
	}
	if detail != "" {
		it.Detail = &detail
	}
	if doc != "" {
		it.Documentation = protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: doc}
	}
	return it
}

// valuesFor are the values that fit the key the cursor is on.
func valuesFor(c cursor, kind fileKind, p *project.Project, path string) []candidate {
	root := filepath.Dir(path)
	if projectFile, ok := findProjectFile(path); ok {
		root = filepath.Dir(projectFile)
	}
	var out []candidate

	// Fixed values from the schema: booleans, sample rates, time signatures.
	if k, ok := findKey(keysFor(kind, c.section, c.fxType), c.key); ok && len(k.values) > 0 {
		for _, v := range k.values {
			out = append(out, candidate{text: strings.Trim(v, `"`), quoted: strings.HasPrefix(v, `"`), doc: k.doc})
		}
		return out
	}

	switch {
	case c.key == "bus" && c.section == "" && kind == kindTrack && p != nil:
		for _, bus := range p.Buses {
			out = append(out, candidate{text: bus.ID, quoted: true, detail: fmt.Sprintf("%.1f dB", bus.Gain), doc: rel(root, bus.SourceFile)})
		}
	case c.key == "file" && c.section == "clip":
		for _, file := range filesIn(root, isAudio) {
			out = append(out, candidate{text: file, quoted: true})
		}
	case c.key == "type" && c.section == "fx":
		names := make([]string, 0, len(effects))
		for name := range effects {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, candidate{text: name, quoted: true, doc: effects[name].doc})
		}
	case c.key == "target" && c.section == "automation":
		out = append(out,
			candidate{text: "gain", quoted: true, detail: "dB", doc: "The track's level."},
			candidate{text: "pan", quoted: true, detail: "-1 to 1", doc: "The track's position in the stereo field."})
		for i, fxType := range fxTypesIn(c.lines) {
			for _, param := range effects[fxType].params {
				out = append(out, candidate{text: fmt.Sprintf("fx[%d].%s", i, param.name), quoted: true, detail: param.detail,
					doc: fmt.Sprintf("%s of the %s.", param.doc, fxType)})
			}
		}
	case c.key == "master" && c.section == "layout":
		for _, file := range filesIn(root, func(p string) bool {
			return filepath.Ext(p) == ".toml" && !strings.Contains(p, "/") && p != "project.toml"
		}) {
			out = append(out, candidate{text: file, quoted: true})
		}
	case (c.key == "tracks" || c.key == "buses") && c.section == "layout" && c.inString:
		for _, file := range filesIn(root, func(p string) bool { return strings.HasPrefix(p, c.key+"/") && filepath.Ext(p) == ".toml" }) {
			out = append(out, candidate{text: file, quoted: true})
		}
	}
	return out
}

// projectOf loads the project a file belongs to, from disk, and says what the file is in it. The kind
// comes from the project's [layout] when it loads, from where the file is otherwise.
func projectOf(path string) (fileKind, *project.Project) {
	projectFile, ok := findProjectFile(path)
	if !ok {
		return kindUnknown, nil
	}
	p, err := project.Load(projectFile)
	if err != nil {
		return kindOf(path), nil
	}
	switch {
	case path == projectFile:
		return kindProject, p
	case p.Master != nil && p.Master.SourceFile == path:
		return kindMaster, p
	}
	for _, bus := range p.Buses {
		if bus.SourceFile == path {
			return kindBus, p
		}
	}
	for _, track := range p.Tracks {
		if track.SourceFile == path {
			return kindTrack, p
		}
	}
	return kindOf(path), p
}

func isAudio(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav", ".wave", ".aif", ".aiff", ".flac", ".mp3", ".ogg":
		return true
	}
	return false
}

// filesIn lists the files under root that match, relative to it, with forward slashes as in the TOML.
func filesIn(root string, match func(rel string) bool) []string {
	var out []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			if r := rel(root, path); match(r) {
				out = append(out, r)
			}
		}
		return nil
	})
	return out
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}

// text is a file's current text: the editor's if it is open, the disk's otherwise.
func (s *state) text(path string) string {
	s.mu.Lock()
	text, ok := s.documents[path]
	s.mu.Unlock()
	if ok {
		return text
	}
	data, _ := os.ReadFile(path)
	return string(data)
}
