package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Clip files are relative to the project's folder, wherever the track file is.
func TestClipFilesResolveFromTheProjectRoot(t *testing.T) {
	for _, trackFile := range []string{"kick.toml", "tracks/kick.toml", "tracks/drums/kick.toml"} {
		root := t.TempDir()
		for name, text := range map[string]string{
			"project.toml": "[project]\nname = \"t\"\n[transport]\nbpm = 120.0\ntime_sig = \"4/4\"\nsample_rate = 48000\nbit_depth = 24\n" +
				"[layout]\ntracks = [\"" + trackFile + "\"]\n",
			trackFile:       "id = \"kick\"\n\n[[clip]]\nfile = \"samples/x.wav\"\nstart = 0.0\nend = 4.0\n\n[[clip]]\nfile = \"samples/nope.wav\"\nstart = 4.0\nend = 8.0\n",
			"samples/x.wav": "x",
		} {
			path := filepath.Join(root, filepath.FromSlash(name))
			os.MkdirAll(filepath.Dir(path), 0o755)
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		p, err := Load(filepath.Join(root, "project.toml"))
		if err != nil {
			t.Fatal(err)
		}
		err = Validate(p)
		if err == nil || strings.Contains(err.Error(), "clip[0]") || !strings.Contains(err.Error(), "clip[1].file — audio file not found") {
			t.Errorf("%s: want only clip[1] reported missing, got: %v", trackFile, err)
		}
	}
}
