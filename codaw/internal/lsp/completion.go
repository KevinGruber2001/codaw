package lsp

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// completionItem adds LSP 3.17's labelDetails to the library's item: text beside the label on
// every row of the list, which is where an effect's type belongs.
type completionItem struct {
	protocol.CompletionItem
	LabelDetails *labelDetails `json:"labelDetails,omitempty"`
}

type labelDetails struct {
	Description string `json:"description,omitempty"`
}

// candidate is one suggestion for a value, without quotes.
type candidate struct {
	text   string
	quoted bool
	detail string
	doc    string
	beside string // shown beside the label on every row
}

type place int

const (
	nowhere place = iota
	onHeader
	onKey
	onValue
)

// cursor is what a position in a file is on, and the text a suggestion there would replace.
type cursor struct {
	place    place
	table    *table
	entry    *entry // the key a value belongs to
	inString bool
	closed   bool // the string has its closing quote
	replace  span
}

func cursorAt(d *document, line, column int) cursor {
	if line < 0 || line >= len(d.lines) {
		return cursor{}
	}
	text := d.lines[line]
	column = min(column, len(text))
	before := text[:column]
	if stripComment(before) != before {
		return cursor{}
	}
	t, e := d.entryAt(line)
	indent := len(before) - len(strings.TrimLeft(before, " \t"))
	c := cursor{table: t}
	switch {
	case e != nil && (e.line < line || column > e.keyAt.end && strings.Contains(before, "=")):
		c.place, c.entry = onValue, e
		from := 0
		if e.line == line {
			from = strings.Index(before, "=") + 1
		}
		var quote byte
		opened := 0
		for i := from; i < column; i++ {
			switch ch := before[i]; {
			case quote == '"' && ch == '\\':
				i++
			case quote != 0:
				if ch == quote {
					quote = 0
				}
			case ch == '"' || ch == '\'':
				quote, opened = ch, i
			}
		}
		if quote != 0 {
			c.inString = true
			c.replace = span{opened + 1, column}
			if i := strings.IndexByte(text[column:], quote); i >= 0 {
				c.closed, c.replace.end = true, column+i
			}
			return c
		}
		c.replace = span{column, column}
		for c.replace.start > from && !strings.ContainsRune(" \t,[=", rune(text[c.replace.start-1])) {
			c.replace.start--
		}
		for c.replace.end < len(text) && !strings.ContainsRune(" \t,]#", rune(text[c.replace.end])) {
			c.replace.end++
		}
	case strings.HasPrefix(before[indent:], "["):
		c.place = onHeader
		c.replace = span{indent, max(column, len(strings.TrimRight(stripComment(text), " \t")))}
	case !strings.ContainsAny(before[indent:], " \t=\"'"):
		c.place = onKey
		c.replace = span{indent, column}
		for c.replace.end < len(text) && isKeyChar(text[c.replace.end]) {
			c.replace.end++
		}
	}
	return c
}

func isKeyChar(ch byte) bool {
	return ch == '_' || ch == '-' || ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

// completion suggests what fits at the cursor: section headers on a "[" line, the keys a section
// may still have, and values: the fixed ones from the schema and the project's own.
func (s *state) completion(ctx *glsp.Context, params *protocol.CompletionParams) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.file(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	line := int(params.Position.Line)
	c := cursorAt(f.doc, line, f.doc.column(line, params.Position.Character))
	replace := f.doc.rng(line, c.replace)

	items := []completionItem{}
	add := func(label, insert string, kind protocol.CompletionItemKind, v candidate) {
		sortText := fmt.Sprintf("%03d", len(items))
		it := completionItem{CompletionItem: protocol.CompletionItem{
			Label:    label,
			Kind:     &kind,
			SortText: &sortText,
			TextEdit: protocol.TextEdit{Range: replace, NewText: insert},
		}}
		if v.detail != "" {
			it.Detail = &v.detail
		}
		if v.doc != "" {
			it.Documentation = protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: v.doc}
		}
		if v.beside != "" {
			filter := label + " " + v.beside
			it.FilterText, it.LabelDetails = &filter, &labelDetails{Description: v.beside}
		}
		items = append(items, it)
	}

	switch c.place {
	case onHeader:
		for _, header := range headers[f.kind] {
			section := strings.Trim(header, "[]")
			array, _ := sectionOf(f.kind, section)
			if own, _, _ := parseHeader(f.doc.lines[line]); !array && len(f.doc.all(section)) > 0 && own != section {
				continue
			}
			if parent, _, nested := strings.Cut(section, "."); nested && !f.hasAbove(parent, line) {
				continue
			}
			add(header, header, protocol.CompletionItemKindModule, candidate{doc: sectionDocs[section]})
		}
	case onKey:
		for _, k := range keysOf(f.kind, c.table) {
			if e := c.table.entry(k.name); e != nil && e.line != line {
				continue
			}
			add(k.name, k.name+" = ", protocol.CompletionItemKindProperty, candidate{detail: k.detail, doc: k.doc})
		}
	case onValue:
		for _, v := range f.values(c) {
			insert := v.text
			switch {
			case v.quoted && !c.inString:
				insert = `"` + v.text + `"`
			case v.quoted && !c.closed:
				insert = v.text + `"`
			}
			add(v.text, insert, protocol.CompletionItemKindValue, v)
		}
	}
	return items, nil
}

// file is one file of a project, as the editor has it, with the project around it.
type file struct {
	path string
	doc  *document
	kind fileKind
	view *view
}

func (s *state) file(uri string) (*file, bool) {
	path, err := uriToPath(uri)
	if err != nil {
		return nil, false
	}
	projectFile, ok := findProjectFile(path)
	if !ok {
		return nil, false
	}
	v := s.load(projectFile)
	f := &file{path: path, doc: v.docs[path], kind: v.kinds[path], view: v}
	if f.doc == nil {
		text, ok := s.source(path)
		if !ok {
			return nil, false
		}
		f.doc, f.kind = parse(text), kindOf(path)
	}
	return f, f.kind != kindUnknown
}

func (f *file) hasAbove(section string, line int) bool {
	for _, t := range f.doc.all(section) {
		if t.line < line {
			return true
		}
	}
	return false
}

// values are the values that fit the key the cursor is on.
func (f *file) values(c cursor) []candidate {
	var out []candidate
	section, name := c.table.name, c.entry.key
	if k, ok := findKey(keysOf(f.kind, c.table), name); ok && len(k.values) > 0 {
		for _, v := range k.values {
			out = append(out, candidate{text: strings.Trim(v, `"`), quoted: strings.HasPrefix(v, `"`), doc: k.doc})
		}
		return out
	}

	root := f.view.root
	switch {
	case name == "bus" && section == "" && f.kind == kindTrack && f.view.project != nil:
		for _, bus := range f.view.project.Buses {
			out = append(out, candidate{text: bus.ID, quoted: true, detail: fmt.Sprintf("%.1f dB", bus.Gain), doc: f.busSummary(bus.ID)})
		}
	case name == "file" && section == "clip":
		for _, path := range filesIn(root, isAudio) {
			out = append(out, candidate{text: path, quoted: true, detail: audioInfo(filepath.Join(root, filepath.FromSlash(path)))})
		}
	case name == "type" && section == "fx":
		for _, effect := range effectNames() {
			out = append(out, candidate{text: effect, quoted: true, doc: effectSummary(effect)})
		}
	case name == "target" && section == "automation":
		out = f.targets()
	case name == "master" && section == "layout":
		for _, path := range filesIn(root, func(p string) bool {
			return filepath.Ext(p) == ".toml" && !strings.Contains(p, "/") && p != "project.toml"
		}) {
			out = append(out, candidate{text: path, quoted: true})
		}
	case (name == "tracks" || name == "buses") && section == "layout":
		for _, path := range filesIn(root, func(p string) bool { return strings.HasPrefix(p, name+"/") && filepath.Ext(p) == ".toml" }) {
			if !strings.Contains(c.entry.value, `"`+path+`"`) {
				out = append(out, candidate{text: path, quoted: true})
			}
		}
	}
	return out
}

// targets are what an automation lane of this track can change: its gain and pan, and every
// parameter of every effect it has. An effect is addressed by position, which says little, so
// each parameter carries its effect's type and shows the effect as it is written in the file.
func (f *file) targets() []candidate {
	d := f.doc
	var out []candidate
	for _, name := range []string{"gain", "pan"} {
		t, _ := resolveTarget(d, name)
		out = append(out, candidate{text: name, quoted: true, detail: t.key.detail, doc: fmt.Sprintf("%s Automates %s, %s.", t.key.doc, t.describe(), t.now(d))})
	}
	for i, fx := range d.all("fx") {
		fxType := fx.text("type")
		for _, param := range effects[fxType].params {
			t, _ := resolveTarget(d, fmt.Sprintf("fx[%d].%s", i, param.name))
			out = append(out, candidate{
				text: t.text, quoted: true, beside: fxType,
				detail: strings.TrimSuffix(fxType+" · "+param.detail, " · "),
				doc:    fmt.Sprintf("%s Automates %s (line %d), %s.\n\n```toml\n%s\n```", param.doc, t.describe(), fx.line+1, t.now(d), d.block(fx)),
			})
		}
	}
	return out
}

func (f *file) busSummary(id string) string {
	if f.view.project == nil {
		return ""
	}
	for _, bus := range f.view.project.Buses {
		if bus.ID != id {
			continue
		}
		summary := fmt.Sprintf("Bus `%s` · %.1f dB · %s", bus.ID, bus.Gain, rel(f.view.root, bus.SourceFile))
		for _, fx := range bus.FX {
			summary += " · " + fx.Type
		}
		return summary
	}
	return ""
}

func effectSummary(name string) string {
	effect, ok := effects[name]
	if !ok {
		return ""
	}
	var md strings.Builder
	md.WriteString(effect.doc)
	for _, param := range effect.params {
		fmt.Fprintf(&md, "\n- `%s`: %s", param.name, param.doc)
		if param.detail != "" {
			fmt.Fprintf(&md, " (%s)", param.detail)
		}
	}
	return md.String()
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
		if d.IsDir() && path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
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
