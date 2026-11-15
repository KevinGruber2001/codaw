package lsp

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/KevinGruber2001/codaw/internal/project"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// view is a project as the editor has it: open files as they are typed, the others from disk.
// One file that does not parse does not hide the rest.
type view struct {
	file    string // project.toml
	root    string
	project *project.Project // the files that parse; nil when project.toml does not
	docs    map[string]*document
	kinds   map[string]fileKind
	found   map[string][]protocol.Diagnostic
	// A bus file is missing or broken: "no such bus" would be a guess.
	busesIncomplete bool
}

func (s *state) load(projectFile string) *view {
	v := &view{
		file: projectFile, root: filepath.Dir(projectFile),
		docs: map[string]*document{}, kinds: map[string]fileKind{}, found: map[string][]protocol.Diagnostic{},
	}
	var p project.Project
	if !v.read(s, projectFile, kindProject, &p) {
		return v
	}
	p.RootDir = v.root
	p.Master = &project.Master{}
	if p.Layout.Master != "" {
		path := v.resolve(p.Layout.Master)
		var master project.Master
		if v.read(s, path, kindMaster, &master) {
			master.SourceFile = path
			p.Master = &master
		} else {
			p.Master = nil
		}
	}
	for _, name := range p.Layout.Buses {
		path := v.resolve(name)
		var bus project.Bus
		if v.read(s, path, kindBus, &bus) {
			bus.SourceFile = path
			p.Buses = append(p.Buses, &bus)
		} else {
			v.busesIncomplete = true
		}
	}
	for _, name := range p.Layout.Tracks {
		path := v.resolve(name)
		var track project.Track
		if v.read(s, path, kindTrack, &track) {
			track.SourceFile = path
			p.Tracks = append(p.Tracks, &track)
		}
	}
	v.project = &p
	v.validate(&p, "")

	// Open files in the project's folders that [layout] does not list.
	for path := range s.documents {
		if v.docs[path] != nil || !strings.HasPrefix(path, v.root+string(filepath.Separator)) {
			continue
		}
		alone := &project.Project{Transport: p.Transport, Buses: p.Buses, RootDir: v.root}
		switch kindOf(path) {
		case kindTrack:
			var track project.Track
			if v.read(s, path, kindTrack, &track) {
				track.SourceFile = path
				alone.Tracks = []*project.Track{&track}
			}
		case kindBus:
			var bus project.Bus
			if v.read(s, path, kindBus, &bus) {
				bus.SourceFile = path
				alone.Buses = []*project.Bus{&bus}
			}
		default:
			continue
		}
		v.validate(alone, path)
		v.add(path, protocol.DiagnosticSeverityWarning, v.docs[path].lineRange(0),
			fmt.Sprintf("%s is not listed in project.toml's [layout]: the engine does not load it", rel(v.root, path)))
	}
	return v
}

func (v *view) resolve(name string) string {
	return filepath.Join(v.root, filepath.FromSlash(name))
}

// read parses one file into dst and notes what is wrong with it. False when there is nothing to
// work with: the file is missing or does not decode.
func (v *view) read(s *state, path string, kind fileKind, dst any) bool {
	text, ok := s.source(path)
	if !ok {
		v.missing(path)
		return false
	}
	d := parse(text)
	v.docs[path], v.kinds[path], v.found[path] = d, kind, nil
	problems, mistyped := lint(d, kind)
	v.found[path] = problems

	var syntax map[string]any
	if _, err := toml.Decode(text, &syntax); err != nil {
		at := errorRange(d, err)
		said := false // a header of the wrong shape is also a syntax error further down; once is enough
		for _, problem := range problems {
			said = said || problem.Range.Start.Line == at.Start.Line && *problem.Severity == protocol.DiagnosticSeverityError
		}
		if !said {
			v.add(path, protocol.DiagnosticSeverityError, at, errorMessage(err))
		}
		return false
	}
	if _, err := toml.Decode(text, dst); err != nil {
		if !mistyped {
			v.add(path, protocol.DiagnosticSeverityError, errorRange(d, err), errorMessage(err))
		}
		return false
	}
	return true
}

// missing reports a file [layout] lists but that is not there, on the line that lists it.
func (v *view) missing(path string) {
	d := v.docs[v.file]
	if d == nil {
		return
	}
	name := rel(v.root, path)
	at := d.lineRange(0)
	for _, t := range d.all("layout") {
		at = d.lineRange(t.line)
		for _, e := range t.entries {
			for line := e.line; line <= e.endLine; line++ {
				if i := strings.Index(d.lines[line], `"`+name+`"`); i >= 0 {
					at = d.rng(line, span{i, i + len(name) + 2})
				}
			}
		}
	}
	v.add(v.file, protocol.DiagnosticSeverityError, at, "file not found: "+name)
}

// validate runs the engine's own validation and places what it finds. only limits it to one file.
func (v *view) validate(p *project.Project, only string) {
	var problems project.ValidationErrors
	if !errors.As(project.Validate(p), &problems) {
		return
	}
	for _, problem := range problems {
		file := problem.File
		if !filepath.IsAbs(file) {
			file = v.file
		}
		d := v.docs[file]
		if d == nil || (only != "" && file != only) {
			continue
		}
		if v.busesIncomplete && problem.Field == "bus" {
			continue
		}
		at, exact := d.locate(problem.Field)
		message := problem.Message
		if !exact {
			segments := strings.Split(problem.Field, ".")
			name, _ := splitIndex(segments[len(segments)-1])
			message = name + ": " + message
		}
		v.add(file, protocol.DiagnosticSeverityError, at, message)
	}
}

func (v *view) add(path string, severity protocol.DiagnosticSeverity, at protocol.Range, message string) {
	source := name
	v.found[path] = append(v.found[path], protocol.Diagnostic{Range: at, Severity: &severity, Source: &source, Message: message})
}

var kindNames = map[string]string{"number": "a number", "string": "a string", "bool": "true or false", "array": "a list"}

// lint checks a file against the schema, from its text alone: sections and keys the engine does
// not know, values of the wrong type, automation targets that lead nowhere. mistyped says a value
// has the wrong type, which the TOML decoder will also trip over, less precisely.
func lint(d *document, kind fileKind) (out []protocol.Diagnostic, mistyped bool) {
	add := func(severity protocol.DiagnosticSeverity, at protocol.Range, format string, args ...any) {
		source := name
		out = append(out, protocol.Diagnostic{Range: at, Severity: &severity, Source: &source, Message: fmt.Sprintf(format, args...)})
	}
	automated := map[string]int{}
	for _, t := range d.tables {
		if t.line >= 0 {
			array, ok := sectionOf(kind, t.name)
			if !ok {
				var names []string
				for _, header := range headers[kind] {
					names = append(names, strings.Trim(header, "[]"))
				}
				add(protocol.DiagnosticSeverityWarning, d.lineRange(t.line), "unknown section %q: the engine ignores it%s", t.name, didYouMean(t.name, names))
				continue
			}
			if array != t.array {
				mistyped = true
				if array {
					add(protocol.DiagnosticSeverityError, d.lineRange(t.line), "%s is a list: write [[%s]]", t.name, t.name)
				} else {
					add(protocol.DiagnosticSeverityError, d.lineRange(t.line), "there is only one %s: write [%s]", t.name, t.name)
				}
				continue
			}
		}
		keys := keysOf(kind, t)
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = k.name
		}
		fxType := t.text("type")
		for _, e := range t.entries {
			k, ok := findKey(keys, e.key)
			switch {
			case ok:
				if got := valueKind(e.value); got != "" && got != k.kind {
					mistyped = true
					add(protocol.DiagnosticSeverityError, d.entryRange(e), "%s must be %s, not %s", e.key, kindNames[k.kind], kindNames[got])
				}
			case t.name != "fx":
				add(protocol.DiagnosticSeverityWarning, d.rng(e.line, e.keyAt), "unknown key %q: the engine ignores it%s", e.key, didYouMean(e.key, names))
			case len(keys) > 1:
				add(protocol.DiagnosticSeverityWarning, d.rng(e.line, e.keyAt), "%s has no parameter %q%s", fxType, e.key, didYouMean(e.key, names))
			}
		}
		switch t.name {
		case "fx":
			if _, ok := effects[fxType]; !ok && fxType != "" {
				add(protocol.DiagnosticSeverityWarning, d.entryRange(t.entry("type")), "unknown effect %q: the engine skips it%s", fxType, didYouMean(fxType, effectNames()))
			}
		case "automation":
			e := t.entry("target")
			if e == nil || valueKind(e.value) != "string" || unquote(e.value) == "" {
				continue
			}
			text := unquote(e.value)
			if _, err := resolveTarget(d, text); err != nil {
				severity := protocol.DiagnosticSeverityError
				if strings.Contains(err.Error(), "has no parameter") {
					severity = protocol.DiagnosticSeverityWarning
				}
				add(severity, d.entryRange(e), "%s", err.Error())
			} else if line, ok := automated[text]; ok {
				add(protocol.DiagnosticSeverityWarning, d.entryRange(e), "%q is already automated by the lane on line %d", text, line+1)
			}
			if _, ok := automated[text]; !ok {
				automated[text] = t.line
			}
		}
	}
	return out, mistyped
}

func didYouMean(name string, candidates []string) string {
	if near := closest(name, candidates); near != "" && near != name {
		return fmt.Sprintf(" (did you mean %q?)", near)
	}
	return ""
}

var decodeErrorPattern = regexp.MustCompile(`^toml: line (\d+)(?: \(last key "[^"]*"\))?: (.*)$`)

// errorRange places what the TOML library reports: a syntax error at its position, a value it
// cannot decode on its line.
func errorRange(d *document, err error) protocol.Range {
	var syntax toml.ParseError
	if errors.As(err, &syntax) {
		line := syntax.Position.Line - 1
		if line < 0 || line >= len(d.lines) {
			return d.lineRange(len(d.lines) - 1)
		}
		start := min(max(syntax.Position.Col-1, 0), len(d.lines[line]))
		end := min(start+max(syntax.Position.Len, 1), len(d.lines[line]))
		if start >= end {
			return d.lineRange(line)
		}
		return d.rng(line, span{start, end})
	}
	if m := decodeErrorPattern.FindStringSubmatch(err.Error()); m != nil {
		line, _ := strconv.Atoi(m[1])
		return d.lineRange(line - 1)
	}
	return d.lineRange(0)
}

func errorMessage(err error) string {
	var syntax toml.ParseError
	if errors.As(err, &syntax) {
		return syntax.Message
	}
	if m := decodeErrorPattern.FindStringSubmatch(err.Error()); m != nil {
		return m[2]
	}
	return err.Error()
}

// kindOf decides what a file is from where it is, for files [layout] does not list.
func kindOf(path string) fileKind {
	switch base := filepath.Base(path); {
	case base == "project.toml":
		return kindProject
	case base == "master.toml":
		return kindMaster
	case filepath.Base(filepath.Dir(path)) == "buses":
		return kindBus
	case filepath.Base(filepath.Dir(path)) == "tracks":
		return kindTrack
	}
	return kindUnknown
}

// findProjectFile walks up from a file to the project.toml it belongs to.
func findProjectFile(path string) (string, bool) {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "project.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", false
		}
	}
}

var drivePattern = regexp.MustCompile(`^/[A-Za-z]:`)

func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("not a file: %s", uri)
	}
	path := u.Path
	if drivePattern.MatchString(path) {
		path = path[1:]
	}
	return filepath.Clean(filepath.FromSlash(path)), nil
}

func pathToURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}
