package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// copyProject copies testdata/basic into a temporary folder, so tests can write a track file freely.
func copyProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := "../../testdata/basic"
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(root, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

const track = `id   = "lead"
bus  = "drums"
gain = -3.0

[[fx]]
type = "reverb"
wet  = 0.3

[[clip]]
file  = "samples/kick_01.wav"
start = 0.0
end   = 4.0

[[automation]]
target = "gain"
`

// labelsAt returns the completion labels for text, with the cursor at the end of the given line.
func labelsAt(t *testing.T, s *state, path, text string, line int) []string {
	t.Helper()
	s.documents[path] = text
	lines := strings.Split(text, "\n")
	result, err := s.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI(path)},
			Position:     protocol.Position{Line: protocol.UInteger(line), Character: protocol.UInteger(len([]rune(lines[line])))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := result.([]protocol.CompletionItem)
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.Label
	}
	return labels
}

func TestCompletion(t *testing.T) {
	root := copyProject(t)
	path := filepath.Join(root, "tracks", "kick.toml")
	s := &state{documents: map[string]string{}}

	tests := []struct {
		name string
		text string // the track file, cursor at the end of the line marked with |
		want []string
		not  []string
	}{
		{"keys a track still lacks", "id = \"lead\"\ngain = 0.0\n|", []string{"bus", "pan", "mute", "solo"}, []string{"id", "gain"}},
		{"headers", "id = \"lead\"\n[|", []string{"[[clip]]", "[[fx]]", "[[automation]]"}, nil},
		{"clip keys", "[[clip]]\nfile = \"x.wav\"\n|", []string{"start", "end", "loop", "offset"}, []string{"file"}},
		{"effect parameters of a reverb", "[[fx]]\ntype = \"reverb\"\n|", []string{"room_size", "wet"}, []string{"type", "low_hz"}},
		{"effect types", "[[fx]]\ntype = \"|", []string{"compressor", "eq_3band", "reverb"}, nil},
		{"bus ids", "id = \"lead\"\nbus = \"|", []string{"drums"}, nil},
		{"samples", "[[clip]]\nfile = \"|", []string{"samples/kick_01.wav", "samples/pad_cmaj.wav"}, nil},
		{"booleans", "[[clip]]\nloop = |", []string{"true", "false"}, nil},
		{"automation targets", strings.Replace(track, `target = "gain"`, `target = "|`, 1), []string{"gain", "pan", "fx[0].room_size", "fx[0].wet"}, []string{"fx[0].low_hz"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.text, "\n")
			line := 0
			for i, l := range lines {
				if strings.Contains(l, "|") {
					line = i
				}
			}
			text := strings.Replace(tt.text, "|", "", 1)
			got := labelsAt(t, s, path, text, line)
			for _, want := range tt.want {
				if !contains(got, want) {
					t.Errorf("missing %q in %v", want, got)
				}
			}
			for _, not := range tt.not {
				if contains(got, not) {
					t.Errorf("should not suggest %q: %v", not, got)
				}
			}
		})
	}
}

func TestHover(t *testing.T) {
	root := copyProject(t)
	path := filepath.Join(root, "tracks", "kick.toml")
	s := &state{documents: map[string]string{path: track}}
	hoverOn := func(line int) string {
		h, err := s.hover(nil, &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI(path)},
			Position:     protocol.Position{Line: protocol.UInteger(line), Character: 1},
		}})
		if err != nil || h == nil {
			return ""
		}
		return h.Contents.(protocol.MarkupContent).Value
	}
	for _, tt := range []struct {
		line int
		want []string
	}{
		{2, []string{"**gain**", "dB, -60 to 6"}},
		{1, []string{"**bus**", "`drums`", "compressor"}},
		{5, []string{"**type**", "**reverb**", "`room_size`"}},
		{9, []string{"**file**", "0.230 s", "44100 Hz", "24-bit", "stereo"}},
	} {
		got := hoverOn(tt.line)
		for _, want := range tt.want {
			if !strings.Contains(got, want) {
				t.Errorf("line %d: missing %q in %q", tt.line+1, want, got)
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
