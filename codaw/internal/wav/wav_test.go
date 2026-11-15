package wav

import (
	"os"
	"path/filepath"
	"testing"
)

// Every sample written is in the file, after the 44-byte header.
func TestSamplesFollowTheHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.wav")
	w, err := Create(path, 48000, 1)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, 100)
	for i := range samples {
		samples[i] = float32(i+1) / 200
	}
	if err := w.WriteFloat32(samples); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if len(data) != headerSize+200 {
		t.Fatalf("file is %d bytes, want %d", len(data), headerSize+200)
	}
	if string(data[0:4]) != "RIFF" || string(data[36:40]) != "data" {
		t.Fatal("no header at the start")
	}
	first := int16(uint16(data[44]) | uint16(data[45])<<8)
	if want := int16(samples[0] * 32767); first != want {
		t.Errorf("first sample is %d, want %d", first, want)
	}
}
