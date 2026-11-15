package lsp

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// document is a TOML file read line by line, without a parser: it has to make sense of files that
// are half-typed. Columns are byte offsets into a line; rng converts them for the editor.
type document struct {
	lines  []string
	tables []*table // tables[0] is the top of the file, before any header
}

type table struct {
	name    string // "", "clip", "automation.point"
	array   bool
	line    int // of the header; -1 for the top of the file
	end     int // the line after its last own line
	index   int // among the tables of its name under the same parent
	parent  *table
	entries []*entry
}

type entry struct {
	key     string
	line    int
	endLine int // after line for an array written over several lines
	keyAt   span
	valueAt span   // on line
	value   string // as written, without comments
}

type span struct{ start, end int }

func parse(text string) *document {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	d := &document{lines: lines}
	current := &table{line: -1}
	d.tables = append(d.tables, current)
	for i := 0; i < len(lines); i++ {
		if name, array, ok := parseHeader(lines[i]); ok {
			current.end = i
			current = &table{name: name, array: array, line: i, parent: d.parentOf(name)}
			for _, t := range d.tables {
				if t.name == name && t.parent == current.parent {
					current.index++
				}
			}
			d.tables = append(d.tables, current)
			continue
		}
		e := parseEntry(lines[i], i)
		if e == nil {
			continue
		}
		for depth := bracketDepth(e.value); depth > 0 && e.endLine+1 < len(lines); {
			if _, _, ok := parseHeader(lines[e.endLine+1]); ok {
				break
			}
			e.endLine++
			more := stripComment(lines[e.endLine])
			e.value += "\n" + more
			depth += bracketDepth(more)
		}
		i = e.endLine
		current.entries = append(current.entries, e)
	}
	current.end = len(lines)
	return d
}

func (d *document) parentOf(name string) *table {
	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return nil
	}
	for i := len(d.tables) - 1; i > 0; i-- {
		if d.tables[i].name == name[:dot] {
			return d.tables[i]
		}
	}
	return nil
}

func parseHeader(line string) (name string, array bool, ok bool) {
	line = strings.TrimSpace(stripComment(line))
	switch {
	case len(line) > 4 && strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]"):
		name, array = line[2:len(line)-2], true
	case len(line) > 2 && strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
		name = line[1 : len(line)-1]
	default:
		return "", false, false
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `[]"=,`) {
		return "", false, false
	}
	return name, array, true
}

func parseEntry(line string, at int) *entry {
	code := stripComment(line)
	eq := strings.Index(code, "=")
	if eq < 0 || strings.HasPrefix(strings.TrimSpace(code), "[") {
		return nil
	}
	key := strings.TrimSpace(code[:eq])
	if key == "" {
		return nil
	}
	keyStart := strings.Index(code, key)
	value := strings.TrimSpace(code[eq+1:])
	valueStart := len(strings.TrimRight(code, " \t")) - len(value)
	return &entry{
		key: key, line: at, endLine: at, value: value,
		keyAt:   span{keyStart, keyStart + len(key)},
		valueAt: span{valueStart, valueStart + len(value)},
	}
}

// stripComment cuts a line at the "#" that starts a comment, leaving those inside strings.
func stripComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return line[:i]
		}
	}
	return line
}

func bracketDepth(code string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(code); i++ {
		switch c := code[i]; {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[':
			depth++
		case c == ']':
			depth--
		}
	}
	return depth
}

func (d *document) tableAt(line int) *table {
	found := d.tables[0]
	for _, t := range d.tables[1:] {
		if t.line <= line {
			found = t
		}
	}
	return found
}

func (d *document) entryAt(line int) (*table, *entry) {
	t := d.tableAt(line)
	for _, e := range t.entries {
		if e.line <= line && line <= e.endLine {
			return t, e
		}
	}
	return t, nil
}

func (d *document) all(name string) []*table {
	var out []*table
	for _, t := range d.tables[1:] {
		if t.name == name {
			out = append(out, t)
		}
	}
	return out
}

func (t *table) entry(key string) *entry {
	for _, e := range t.entries {
		if e.key == key {
			return e
		}
	}
	return nil
}

// text is the value of a string key, without its quotes; "" when it is not a string.
func (t *table) text(key string) string {
	if e := t.entry(key); e != nil && valueKind(e.value) == "string" {
		return unquote(e.value)
	}
	return ""
}

// block is a table as written, header to last line, without the blank lines after it.
func (d *document) block(t *table) string {
	from, to := max(t.line, 0), t.end
	for to > from+1 && strings.TrimSpace(d.lines[to-1]) == "" {
		to--
	}
	return strings.Join(d.lines[from:to], "\n")
}

// locate finds a validation error's field: "gain", "transport.bpm", "clip[1].end",
// "automation[0].point[2].beat". The value if the key is there (exact), otherwise the closest
// place that is: its table's header, or the first line.
func (d *document) locate(field string) (r protocol.Range, exact bool) {
	segments := strings.Split(field, ".")
	t, name := d.tables[0], ""
	for _, segment := range segments[:len(segments)-1] {
		part, index := splitIndex(segment)
		var parent *table
		if name != "" {
			part, parent = name+"."+part, t
		}
		var next *table
		for _, candidate := range d.all(part) {
			if candidate.parent == parent && candidate.index == index {
				next = candidate
			}
		}
		if next == nil {
			return d.lineRange(max(t.line, 0)), false
		}
		t, name = next, part
	}
	key, _ := splitIndex(segments[len(segments)-1])
	if e := t.entry(key); e != nil {
		return d.entryRange(e), true
	}
	return d.lineRange(max(t.line, 0)), false
}

// splitIndex splits "clip[2]" into "clip", 2; "gain" into "gain", 0.
func splitIndex(segment string) (string, int) {
	open := strings.Index(segment, "[")
	if open < 0 || !strings.HasSuffix(segment, "]") {
		return segment, 0
	}
	index, err := strconv.Atoi(segment[open+1 : len(segment)-1])
	if err != nil {
		return segment, 0
	}
	return segment[:open], index
}

// entryRange is an entry's value, or its key when it has none yet.
func (d *document) entryRange(e *entry) protocol.Range {
	if e.endLine > e.line {
		return protocol.Range{
			Start: d.position(e.line, e.valueAt.start),
			End:   d.position(e.endLine, len(strings.TrimRight(stripComment(d.lines[e.endLine]), " \t"))),
		}
	}
	if e.valueAt.start == e.valueAt.end {
		return d.rng(e.line, e.keyAt)
	}
	return d.rng(e.line, e.valueAt)
}

func (d *document) rng(line int, at span) protocol.Range {
	return protocol.Range{Start: d.position(line, at.start), End: d.position(line, at.end)}
}

// lineRange spans a line's text, without the indentation.
func (d *document) lineRange(line int) protocol.Range {
	if line < 0 || line >= len(d.lines) {
		return protocol.Range{}
	}
	text := d.lines[line]
	return d.rng(line, span{len(text) - len(strings.TrimLeft(text, " \t")), len(text)})
}

// position converts a byte column into the editor's, which counts UTF-16 code units.
func (d *document) position(line, column int) protocol.Position {
	if line < 0 || line >= len(d.lines) {
		return protocol.Position{}
	}
	text := d.lines[line]
	units := 0
	for _, r := range text[:min(max(column, 0), len(text))] {
		units += max(utf16.RuneLen(r), 1)
	}
	return protocol.Position{Line: protocol.UInteger(line), Character: protocol.UInteger(units)}
}

// column converts the editor's column on a line into a byte offset.
func (d *document) column(line int, character protocol.UInteger) int {
	if line < 0 || line >= len(d.lines) {
		return 0
	}
	units := 0
	for i, r := range d.lines[line] {
		if units >= int(character) {
			return i
		}
		units += max(utf16.RuneLen(r), 1)
	}
	return len(d.lines[line])
}

var numberPattern = regexp.MustCompile(`^[+-]?(\d[\d_]*(\.\d[\d_]*)?([eE][+-]?\d+)?|inf|nan|0x[0-9a-fA-F_]+|0o[0-7_]+|0b[01_]+)$`)

// valueKind is "string", "number", "bool" or "array"; "" for anything else, which includes a
// value that is still being typed.
func valueKind(value string) string {
	switch {
	case value == "":
		return ""
	case value[0] == '"' || value[0] == '\'':
		if len(value) >= 2 && value[len(value)-1] == value[0] {
			return "string"
		}
	case value[0] == '[':
		return "array"
	case value == "true" || value == "false":
		return "bool"
	case numberPattern.MatchString(value):
		return "number"
	}
	return ""
}

func unquote(value string) string {
	if s, err := strconv.Unquote(value); err == nil && strings.HasPrefix(value, `"`) {
		return s
	}
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return value
}

var stringPattern = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'([^']*)'`)

// stringAt is the string literal around a column of a line: its text and where it is.
func stringAt(line string, column int) (string, span, bool) {
	for _, m := range stringPattern.FindAllStringSubmatchIndex(stripComment(line), -1) {
		if m[0] <= column && column <= m[1] {
			return unquote(line[m[0]:m[1]]), span{m[0], m[1]}, true
		}
	}
	return "", span{}, false
}
