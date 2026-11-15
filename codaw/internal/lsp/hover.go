package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-audio/wav"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var stringValue = regexp.MustCompile(`=\s*"([^"]*)"`)

// hover explains the key under the cursor: what it is, its unit and range. For some values it adds
// what is on the other end: a sample's length and format, a bus's level and effects, an effect's
// parameters.
func (s *state) hover(ctx *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil, nil
	}
	text := s.text(path)
	line := int(params.Position.Line)
	c := cursorAt(text, line, 0)
	if line >= len(c.lines) {
		return nil, nil
	}
	name := keyOf(c.lines[line])
	if name == "" {
		return nil, nil
	}
	kind, p := projectOf(path)
	k, ok := findKey(keysFor(kind, c.section, c.fxType), name)
	if !ok {
		return nil, nil
	}

	var md strings.Builder
	fmt.Fprintf(&md, "**%s**", k.name)
	if k.detail != "" {
		fmt.Fprintf(&md, " · %s", k.detail)
	}
	fmt.Fprintf(&md, "\n\n%s", k.doc)

	value := ""
	if m := stringValue.FindStringSubmatch(c.lines[line]); m != nil {
		value = m[1]
	}
	switch {
	case name == "file" && c.section == "clip" && value != "":
		if projectFile, ok := findProjectFile(path); ok {
			md.WriteString("\n\n" + audioInfo(filepath.Join(filepath.Dir(projectFile), filepath.FromSlash(value))))
		}
	case name == "bus" && value != "" && p != nil:
		for _, bus := range p.Buses {
			if bus.ID == value {
				fmt.Fprintf(&md, "\n\n→ `%s` · %.1f dB", value, bus.Gain)
				for _, fx := range bus.FX {
					fmt.Fprintf(&md, " · %s", fx.Type)
				}
			}
		}
	case name == "type" && c.section == "fx":
		if effect, ok := effects[value]; ok {
			fmt.Fprintf(&md, "\n\n**%s**: %s", value, effect.doc)
			for _, param := range effect.params {
				fmt.Fprintf(&md, "\n- `%s`: %s", param.name, param.doc)
				if param.detail != "" {
					fmt.Fprintf(&md, " (%s)", param.detail)
				}
			}
		}
	}

	r := lineRange(c.lines, line)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: md.String()},
		Range:    &r,
	}, nil
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
	// The length from the audio data itself. The library's Duration() is off for files with extra chunks
	// (LIST, smpl, ...), which samples often have.
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
