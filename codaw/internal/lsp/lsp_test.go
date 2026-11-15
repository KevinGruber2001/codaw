package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

const projectText = `[project]
name = "test"

[transport]
bpm         = 120.0
time_sig    = "4/4"
sample_rate = 48000
bit_depth   = 24

[layout]
master = "master.toml"
buses  = ["buses/drums.toml"]
tracks = [
  "tracks/kick.toml",
]
`

const trackText = `id   = "kick"
bus  = "drums"
gain = -3.0

[[fx]]
type   = "eq_3band"
low_db = 3.0

[[fx]]
type = "reverb"
wet  = 0.3 # tail

[[clip]]
file  = "samples/kick_01.wav"
start = 0.0
end   = 4.0

[[automation]]
target = "gain"

[[automation.point]]
beat  = 0
value = -24.0

[[automation.point]]
beat  = 16
value = -50.0
`

// testProject writes a small project to a temporary folder and returns a server and the folder.
func testProject(t *testing.T) (*state, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sample, err := os.ReadFile("../../testdata/basic/samples/kick_01.wav")
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"project.toml":        projectText,
		"master.toml":         "gain = 0.0\n",
		"buses/drums.toml":    "id = \"drums\"\ngain = -6.0\n\n[[fx]]\ntype = \"compressor\"\n",
		"tracks/kick.toml":    trackText,
		"samples/kick_01.wav": string(sample),
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return newState("test"), root
}

type session struct {
	t         *testing.T
	s         *state
	root      string
	published map[string][]string
}

func newSession(t *testing.T) *session {
	s, root := testProject(t)
	return &session{t: t, s: s, root: root, published: map[string][]string{}}
}

func (x *session) path(name string) string { return filepath.Join(x.root, filepath.FromSlash(name)) }

func (x *session) notify(method string, params any) {
	p := params.(protocol.PublishDiagnosticsParams)
	path, _ := uriToPath(p.URI)
	lines := []string{}
	for _, d := range p.Diagnostics {
		severity := "error"
		if *d.Severity == protocol.DiagnosticSeverityWarning {
			severity = "warning"
		}
		lines = append(lines, fmt.Sprintf("%d:%d-%d %s: %s", d.Range.Start.Line+1, d.Range.Start.Character, d.Range.End.Character, severity, d.Message))
	}
	x.published[rel(x.root, path)] = lines
}

// edit is the editor sending a file's new text.
func (x *session) edit(name, text string) {
	x.s.update(x.notify, pathToURI(x.path(name)), func(path string) {
		x.s.documents[path] = &openDocument{uri: pathToURI(path), text: text}
	})
}

func (x *session) save(name, text string) {
	if err := os.WriteFile(x.path(name), []byte(text), 0o644); err != nil {
		x.t.Fatal(err)
	}
}

func (x *session) expect(name string, want ...string) {
	x.t.Helper()
	got, ok := x.published[name]
	if !ok {
		x.t.Fatalf("%s: nothing published", name)
	}
	if len(got) != len(want) {
		x.t.Fatalf("%s: got %d problems, want %d:\n%s", name, len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.Contains(got[i], want[i]) {
			x.t.Errorf("%s: problem %d is %q, want it to contain %q", name, i, got[i], want[i])
		}
	}
}

// A fixed value has to clear its error, whatever is on disk: an autosave of the broken text used
// to keep the error up after the text was fixed.
func TestFixedTextClearsTheError(t *testing.T) {
	x := newSession(t)
	broken := strings.Replace(trackText, "-50.0", "-50.xxx", 1)
	x.edit("tracks/kick.toml", broken)
	x.save("tracks/kick.toml", broken)
	x.expect("tracks/kick.toml", "27:8-12 error: Invalid float")

	x.edit("tracks/kick.toml", strings.Replace(trackText, "-50.0", "-50.", 1))
	x.expect("tracks/kick.toml", "27:8-12 error: Invalid float")

	x.edit("tracks/kick.toml", strings.Replace(trackText, "-50.0", "-50.00", 1))
	x.expect("tracks/kick.toml")
}

func TestDiagnostics(t *testing.T) {
	replace := func(old, new string) string {
		if !strings.Contains(trackText, old) {
			t.Fatalf("the track has no %q", old)
		}
		return strings.Replace(trackText, old, new, 1)
	}
	for _, tt := range []struct {
		name string
		text string
		want []string
	}{
		{"clean", trackText, nil},
		{"out of range, on the value", replace("gain = -3.0", "gain = -70.0"), []string{"3:7-12 error: gain must be between"}},
		{"wrong type", replace("gain = -3.0", `gain = "loud"`), []string{"3:7-13 error: gain must be a number, not a string"}},
		{"unknown key", replace("gain = -3.0", "gian = -3.0"), []string{`3:0-4 warning: unknown key "gian": the engine ignores it (did you mean "gain"?)`}},
		{"unknown effect", replace(`"reverb"`, `"revrb"`), []string{`10:7-14 warning: unknown effect "revrb": the engine skips it (did you mean "reverb"?)`}},
		{"parameter of another effect", replace("low_db = 3.0", "wet = 1"), []string{`7:0-3 warning: eq_3band has no parameter "wet"`}},
		{"missing key, on the header", replace("file  = \"samples/kick_01.wav\"\n", ""), []string{"13:0-8 error: file: required"}},
		{"missing sample", replace("kick_01", "nope"), []string{"14:8-26 error: audio file not found"}},
		{"unknown bus", replace(`"drums"`, `"drms"`), []string{`2:7-13 error: references bus "drms"`}},
		{"table for a list", replace("[[clip]]", "[clip]"), []string{"13:0-6 error: clip is a list: write [[clip]]"}},
		{"unknown section", replace("[[clip]]", "[[clap]]"), []string{`13:0-8 warning: unknown section "clap"`}},
		{"target: no such effect", replace(`target = "gain"`, `target = "fx[2].wet"`), []string{"19:9-20 error: fx[2] does not exist: this track has 2 effects"}},
		{"target: no such parameter", replace(`target = "gain"`, `target = "fx[0].wet"`), []string{`19:9-20 warning: eq_3band has no parameter "wet"`}},
		{"target: nonsense", replace(`target = "gain"`, `target = "volume"`), []string{`19:9-17 error: unknown target "volume"`}},
		{"target: effect parameter", replace(`target = "gain"`, `target = "fx[1].wet"`), nil},
		{"target: twice", trackText + "\n[[automation]]\ntarget = \"gain\"\n[[automation.point]]\nbeat = 1\nvalue = 0\n", []string{`30:9-15 warning: "gain" is already automated by the lane on line 18`}},
		{"nested field", replace("beat  = 16", "beat  = -1"), []string{"26:8-10 error: must be >= 0"}},
		{"columns count UTF-16 units", replace(`bus  = "drums"`, `bus  = "drms" # 🎛`) + "", []string{"2:7-13 error"}},
		{"windows line endings", strings.ReplaceAll(replace("gain = -3.0", "gain = -70.0"), "\n", "\r\n"), []string{"3:7-12 error: gain must be between"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newSession(t)
			x.edit("tracks/kick.toml", tt.text)
			x.expect("tracks/kick.toml", tt.want...)
		})
	}
}

func TestProjectDiagnostics(t *testing.T) {
	x := newSession(t)
	x.edit("project.toml", strings.NewReplacer("120.0", "999", `"tracks/kick.toml"`, `"tracks/kik.toml"`).Replace(projectText))
	x.expect("project.toml", "5:14-17 error: must be between 20 and 300", "14:2-19 error: file not found: tracks/kik.toml")

	// While project.toml does not parse, what is known about the other files stays.
	x.edit("project.toml", projectText)
	x.edit("tracks/kick.toml", strings.Replace(trackText, "-3.0", "-70.0", 1))
	x.edit("project.toml", strings.Replace(projectText, "120.0", "12x", 1))
	x.expect("project.toml", "5:16-17 error")
	x.expect("tracks/kick.toml", "gain must be between")

	// A bus that does not parse is not a bus that does not exist.
	x.edit("project.toml", projectText)
	x.edit("tracks/kick.toml", trackText)
	x.edit("buses/drums.toml", `id = "drums`)
	x.expect("buses/drums.toml", "1:10-11 error")
	x.expect("tracks/kick.toml")

	// Another open file's unsaved text counts: the bus was renamed in the editor only.
	x.edit("buses/drums.toml", "id = \"perc\"\n")
	x.expect("tracks/kick.toml", `references bus "drums"`)

	// A file in tracks/ that [layout] does not list is checked, and said to be unused.
	x.save("tracks/new.toml", "")
	x.edit("tracks/new.toml", "id = \"new\"\ngain = 99\n")
	x.expect("tracks/new.toml", "1:0-10 warning: tracks/new.toml is not listed", "2:7-9 error: gain must be between")
}

func (x *session) complete(name, text string) []completionItem {
	x.t.Helper()
	at := strings.Index(text, "|")
	if at < 0 {
		x.t.Fatal("no | in the text")
	}
	before := text[:at]
	x.edit(name, before+text[at+1:])
	result, err := x.s.completion(nil, &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI(x.path(name))},
		Position: protocol.Position{
			Line:      protocol.UInteger(strings.Count(before, "\n")),
			Character: protocol.UInteger(len(before) - strings.LastIndex(before, "\n") - 1),
		},
	}})
	if err != nil {
		x.t.Fatal(err)
	}
	items, _ := result.([]completionItem)
	return items
}

func TestCompletion(t *testing.T) {
	x := newSession(t)
	target := func(value string) string { return strings.Replace(trackText, `target = "gain"`, "target = "+value, 1) }
	for _, tt := range []struct {
		name string
		file string
		text string
		want []string // labels, in order; all of them unless it ends with "…"
		edit string   // what the first suggestion inserts, as "text@from-to"
	}{
		{"keys a track still lacks", "tracks/kick.toml", "id = \"lead\"\ngain = 0.0\n|", []string{"bus", "pan", "mute", "solo"}, "bus = @0-0"},
		{"a key being retyped", "tracks/kick.toml", "id = \"lead\"\nga|in = 0.0\n", []string{"bus", "gain", "pan", "mute", "solo"}, "bus = @0-4"},
		{"headers", "tracks/kick.toml", "id = \"lead\"\n[|]", []string{"[[clip]]", "[[fx]]", "[[automation]]"}, "[[clip]]@0-2"},
		{"points only under a lane", "tracks/kick.toml", trackText + "[[|", []string{"[[clip]]", "[[fx]]", "[[automation]]", "[[automation.point]]"}, ""},
		{"clip keys", "tracks/kick.toml", "[[clip]]\nfile = \"x.wav\"\n|", []string{"start", "end", "offset", "loop", "gain"}, ""},
		{"an effect without a type", "tracks/kick.toml", "[[fx]]\n|", []string{"type"}, ""},
		{"a reverb's parameters", "tracks/kick.toml", "[[fx]]\ntype = \"reverb\"\n|", []string{"room_size", "wet"}, ""},
		{"effect types", "tracks/kick.toml", "[[fx]]\ntype = \"|\"", []string{"compressor", "eq_3band", "reverb"}, "compressor@8-8"},
		{"buses, unquoted", "tracks/kick.toml", "bus = |", []string{"drums"}, `"drums"@6-6`},
		{"buses, unclosed string", "tracks/kick.toml", `bus = "dr|`, []string{"drums"}, `drums"@7-9`},
		{"samples", "tracks/kick.toml", "[[clip]]\nfile = \"sam|\"", []string{"samples/kick_01.wav"}, "samples/kick_01.wav@8-11"},
		{"booleans replace the word", "tracks/kick.toml", "[[clip]]\nloop = tr|ue # x", []string{"true", "false"}, "true@7-11"},
		{"nothing in a comment", "tracks/kick.toml", "id = \"x\" # see |", nil, ""},
		{"nothing for a free value", "tracks/kick.toml", "[[automation.point]]\nvalue = |", nil, ""},
		{"targets", "tracks/kick.toml", target(`"|"`), []string{"gain", "pan", "fx[0].low_hz", "fx[0].low_db", "fx[0].mid_hz", "fx[0].mid_db", "fx[0].high_hz", "fx[0].high_db", "fx[1].room_size", "fx[1].wet"}, "gain@10-10"},
		{"targets, unquoted", "tracks/kick.toml", target("|"), []string{"gain", "…"}, `"gain"@9-9`},
		{"tracks not listed yet", "project.toml", strings.Replace(projectText, "\"tracks/kick.toml\",\n", "\"tracks/kick.toml\",\n  \"|\"\n", 1), nil, ""},
		{"buses in an empty list", "project.toml", strings.Replace(projectText, `["buses/drums.toml"]`, "[|]", 1), []string{"buses/drums.toml"}, `"buses/drums.toml"@10-10`},
		{"sections a project lacks", "project.toml", strings.Replace(projectText, "[layout]", "[|", 1), []string{"[layout]"}, ""},
		{"fixed values", "project.toml", strings.Replace(projectText, "48000", "|", 1), []string{"44100", "48000", "88200", "96000"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := x.complete(tt.file, tt.text)
			var got []string
			for _, item := range items {
				got = append(got, item.Label)
			}
			want := tt.want
			if len(want) > 0 && want[len(want)-1] == "…" {
				want = want[:len(want)-1]
				got = got[:min(len(got), len(want))]
			}
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Fatalf("got %v, want %v", got, want)
			}
			if tt.edit != "" {
				edit := items[0].TextEdit.(protocol.TextEdit)
				if got := fmt.Sprintf("%s@%d-%d", edit.NewText, edit.Range.Start.Character, edit.Range.End.Character); got != tt.edit {
					t.Errorf("the first suggestion inserts %s, want %s", got, tt.edit)
				}
			}
		})
	}
}

// An effect's parameter says which effect it belongs to: the type beside it, the block in its
// documentation.
func TestTargetSuggestionsShowTheirEffect(t *testing.T) {
	x := newSession(t)
	items := x.complete("tracks/kick.toml", strings.Replace(trackText, `target = "gain"`, `target = "|"`, 1))
	var wet completionItem
	for _, item := range items {
		if item.Label == "fx[1].wet" {
			wet = item
		}
	}
	if wet.LabelDetails == nil || wet.LabelDetails.Description != "reverb" {
		t.Errorf("beside the label: %+v, want reverb", wet.LabelDetails)
	}
	if wet.Detail == nil || *wet.Detail != "reverb · 0 to 1, default 0.3" {
		t.Errorf("detail: %v", wet.Detail)
	}
	doc := wet.Documentation.(protocol.MarkupContent).Value
	for _, want := range []string{"`wet` of the 2nd effect, `reverb` (line 9)", "now `0.3`", "```toml\n[[fx]]\ntype = \"reverb\"\nwet  = 0.3 # tail\n```"} {
		if !strings.Contains(doc, want) {
			t.Errorf("documentation lacks %q:\n%s", want, doc)
		}
	}
}

func TestHover(t *testing.T) {
	x := newSession(t)
	for _, tt := range []struct {
		name string
		file string
		text string // hovering at |
		at   string // the range, as "line:from-to"
		want []string
	}{
		{"a key", "tracks/kick.toml", strings.Replace(trackText, "gain = -3.0", "ga|in = -3.0", 1), "3:0-4", []string{"**gain** · dB, -60 to 6", "Automated by the lane on line 18"}},
		{"a bus", "tracks/kick.toml", strings.Replace(trackText, `"drums"`, `"dr|ums"`, 1), "2:7-14", []string{"Bus `drums` · -6.0 dB · buses/drums.toml · compressor"}},
		{"a section", "tracks/kick.toml", strings.Replace(trackText, "[[clip]]", "[[cl|ip]]", 1), "13:0-8", []string{"**[[clip]]**", "A piece of audio"}},
		{"an effect type", "tracks/kick.toml", strings.Replace(trackText, `"reverb"`, `"rev|erb"`, 1), "10:7-15", []string{"**reverb**", "`room_size`"}},
		{"a sample", "tracks/kick.toml", strings.Replace(trackText, "kick_01", "kick|_01", 1), "14:8-29", []string{"0.230 s", "44100 Hz", "24-bit", "stereo"}},
		{"a beat", "tracks/kick.toml", strings.Replace(trackText, "end   = 4.0", "end   = 4|.0", 1), "16:8-11", []string{"Beat 4.0 is at 2.000 s, at 120 BPM."}},
		{"a target", "tracks/kick.toml", strings.Replace(trackText, `"gain"`, `"fx[1].w|et"`, 1), "19:9-20", []string{"`wet` of the 2nd effect, `reverb` (line 9), now `0.3`", "```toml\n[[fx]]\ntype = \"reverb\""}},
		{"a target that leads nowhere", "tracks/kick.toml", strings.Replace(trackText, `"gain"`, `"fx[4].w|et"`, 1), "19:9-20", []string{"⚠ fx[4] does not exist"}},
		{"a point's value", "tracks/kick.toml", strings.Replace(trackText, "value = -24.0", "value = -2|4.0", 1), "23:8-13", []string{"Sets the track's `gain` · dB, -60 to 6."}},
		{"a listed file", "project.toml", strings.Replace(projectText, "tracks/kick", "tracks/ki|ck", 1), "14:2-20", []string{"Track `kick` → bus `drums` · -3.0 dB · 1 clip · 2 effects · 1 automation lane"}},
		{"a comment", "tracks/kick.toml", strings.Replace(trackText, "# tail", "# ta|il", 1), "", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			at := strings.Index(tt.text, "|")
			before := tt.text[:at]
			x.edit(tt.file, before+tt.text[at+1:])
			h, err := x.s.hover(nil, &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI(x.path(tt.file))},
				Position: protocol.Position{
					Line:      protocol.UInteger(strings.Count(before, "\n")),
					Character: protocol.UInteger(len(before) - strings.LastIndex(before, "\n") - 1),
				},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if tt.at == "" {
				if h != nil {
					t.Fatalf("got a hover, want none: %v", h.Contents)
				}
				return
			}
			if h == nil {
				t.Fatal("no hover")
			}
			if got := fmt.Sprintf("%d:%d-%d", h.Range.Start.Line+1, h.Range.Start.Character, h.Range.End.Character); got != tt.at {
				t.Errorf("range %s, want %s", got, tt.at)
			}
			md := h.Contents.(protocol.MarkupContent).Value
			for _, want := range tt.want {
				if !strings.Contains(md, want) {
					t.Errorf("lacks %q:\n%s", want, md)
				}
			}
		})
	}
}

func TestURIs(t *testing.T) {
	for uri, want := range map[string]string{
		"file:///home/k/my%20song/project.toml": "/home/k/my song/project.toml",
		"file:///home/k/tracks/../project.toml": "/home/k/project.toml",
	} {
		if got, err := uriToPath(uri); err != nil || filepath.ToSlash(got) != want {
			t.Errorf("%s: got %q, %v; want %q", uri, got, err, want)
		}
	}
	if _, err := uriToPath("untitled:Untitled-1"); err == nil {
		t.Error("a file that is not on disk has no path")
	}
	if got := pathToURI("/home/k/my song/project.toml"); got != "file:///home/k/my%20song/project.toml" {
		t.Errorf("got %s", got)
	}
}
