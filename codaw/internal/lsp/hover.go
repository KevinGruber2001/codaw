package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-audio/wav"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// hover explains what is under the cursor: a section, a key with its unit and range, and for
// some values what is on the other end: a sample's length and format, a bus, an effect, the
// effect block an automation target points at.
func (s *state) hover(ctx *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.file(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	d, line := f.doc, int(params.Position.Line)
	if line >= len(d.lines) {
		return nil, nil
	}
	column := d.column(line, params.Position.Character)
	answer := func(at protocol.Range, md string) (*protocol.Hover, error) {
		return &protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: md}, Range: &at}, nil
	}

	if section, _, ok := parseHeader(d.lines[line]); ok {
		if doc, ok := sectionDocs[section]; ok {
			if _, known := sectionOf(f.kind, section); known {
				return answer(d.lineRange(line), fmt.Sprintf("**%s**\n\n%s", strings.TrimSpace(stripComment(d.lines[line])), doc))
			}
		}
		return nil, nil
	}
	t, e := d.entryAt(line)
	if e == nil || column > len(stripComment(d.lines[line])) {
		return nil, nil
	}
	k, ok := findKey(keysOf(f.kind, t), e.key)
	if !ok {
		return nil, nil
	}

	var md strings.Builder
	fmt.Fprintf(&md, "**%s**", k.name)
	if k.detail != "" {
		fmt.Fprintf(&md, " · %s", k.detail)
	}
	fmt.Fprintf(&md, "\n\n%s", k.doc)
	if more := f.about(t, e, line, column); more != "" {
		md.WriteString("\n\n" + more)
	}

	at := d.rng(line, e.keyAt)
	if e.line < line || column >= e.valueAt.start {
		at = d.entryRange(e)
		if _, where, ok := stringAt(d.lines[line], column); ok && strings.HasPrefix(e.value, "[") {
			at = d.rng(line, where)
		}
	}
	return answer(at, md.String())
}

// about is what a key's value leads to, for the keys whose value is more than itself.
func (f *file) about(t *table, e *entry, line, column int) string {
	d, root, p := f.doc, f.view.root, f.view.project
	value := unquote(e.value)
	switch {
	case t.name == "clip" && e.key == "file" && value != "":
		return audioInfo(filepath.Join(root, filepath.FromSlash(value)))
	case t.name == "clip" && (e.key == "start" || e.key == "end") && p != nil && p.Transport.BPM > 0:
		if beats, err := strconv.ParseFloat(strings.ReplaceAll(e.value, "_", ""), 64); err == nil {
			return fmt.Sprintf("Beat %s is at %.3f s, at %s BPM.", e.value, beats*60/p.Transport.BPM, strconv.FormatFloat(p.Transport.BPM, 'f', -1, 64))
		}
	case t.name == "" && e.key == "bus" && value != "":
		if summary := f.busSummary(value); summary != "" {
			return "→ " + summary
		}
	case t.name == "" && (e.key == "gain" || e.key == "pan") && f.kind == kindTrack:
		for _, lane := range d.all("automation") {
			if lane.text("target") == e.key {
				return fmt.Sprintf("Automated by the lane on line %d: while playing, the lane's value replaces this one.", lane.line+1)
			}
		}
	case t.name == "fx" && e.key == "type":
		if summary := effectSummary(value); summary != "" {
			return fmt.Sprintf("**%s**: %s", value, summary)
		}
	case t.name == "automation" && e.key == "target" && value != "":
		return targetAbout(d, value)
	case t.name == "automation.point" && e.key == "value" && t.parent != nil:
		if text := t.parent.text("target"); text != "" {
			if target, err := resolveTarget(d, text); err == nil {
				about := "Sets " + target.describe()
				if target.key.detail != "" {
					about += " · " + target.key.detail
				}
				return about + "."
			}
		}
	case t.name == "layout" && p != nil:
		if name, _, ok := stringAt(d.lines[line], column); ok {
			return f.fileSummary(filepath.Join(root, filepath.FromSlash(name)))
		}
	}
	return ""
}

// targetAbout says what an automation target points at; for an effect's parameter, with the
// effect as it is written in the file.
func targetAbout(d *document, text string) string {
	target, err := resolveTarget(d, text)
	if target.fx == nil {
		if err != nil {
			return "⚠ " + err.Error()
		}
		return fmt.Sprintf("→ %s, %s.", target.describe(), target.now(d))
	}
	var md strings.Builder
	fmt.Fprintf(&md, "→ %s (line %d), %s.", target.describe(), target.fx.line+1, target.now(d))
	if target.known {
		fmt.Fprintf(&md, " %s", target.key.doc)
		if target.key.detail != "" {
			fmt.Fprintf(&md, " (%s)", target.key.detail)
		}
	} else if err != nil {
		fmt.Fprintf(&md, "\n\n⚠ %s", err.Error())
	}
	fmt.Fprintf(&md, "\n\n```toml\n%s\n```", d.block(target.fx))
	return md.String()
}

// fileSummary describes a file [layout] lists.
func (f *file) fileSummary(path string) string {
	p := f.view.project
	if p.Master != nil && p.Master.SourceFile == path {
		return fmt.Sprintf("Master · %.1f dB · %s", p.Master.Gain, count(len(p.Master.FX), "effect"))
	}
	for _, bus := range p.Buses {
		if bus.SourceFile == path {
			return f.busSummary(bus.ID)
		}
	}
	for _, track := range p.Tracks {
		if track.SourceFile == path {
			to := "the master"
			if track.Bus != "" {
				to = "bus `" + track.Bus + "`"
			}
			return fmt.Sprintf("Track `%s` → %s · %.1f dB · %s · %s · %s", track.ID, to, track.Gain,
				count(len(track.Clips), "clip"), count(len(track.FX), "effect"), count(len(track.Automation), "automation lane"))
		}
	}
	if _, err := os.Stat(path); err != nil {
		return "⚠ The file does not exist."
	}
	return ""
}

// audioInfo describes an audio file: length and format, or why it cannot be read.
func audioInfo(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "⚠ The file does not exist."
	}
	defer f.Close()
	decoder := wav.NewDecoder(f)
	if !decoder.IsValidFile() {
		return "Not a WAV file the engine can read."
	}
	// The length from the audio data itself: the library's Duration() is off for files with extra
	// chunks (LIST, smpl, ...), which samples often have.
	if err := decoder.FwdToPCM(); err != nil || decoder.NumChans == 0 || decoder.BitDepth == 0 || decoder.SampleRate == 0 {
		return "A WAV file whose length cannot be read."
	}
	frames := decoder.PCMSize / (int(decoder.NumChans) * int(decoder.BitDepth) / 8)
	seconds := float64(frames) / float64(decoder.SampleRate)
	channels := fmt.Sprintf("%d channels", decoder.NumChans)
	switch decoder.NumChans {
	case 1:
		channels = "mono"
	case 2:
		channels = "stereo"
	}
	return fmt.Sprintf("%.3f s · %d Hz · %d-bit · %s", seconds, decoder.SampleRate, decoder.BitDepth, channels)
}
