// Command playtest is a throwaway harness for Phase 3: it boots the audio
// engine and plays one file so we can confirm the cgo wrapper actually makes
// sound. It deliberately does NOT touch internal/project or internal/state —
// this is the smallest possible end-to-end audio test.
//
// Usage:
//
//	go run ./cmd/playtest path/to/file.wav
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/KevinGruber2001/codaw/internal/audio"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: playtest <file.wav>")
		os.Exit(1)
	}
	path := os.Args[1]

	eng, err := audio.NewEngine()
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	defer eng.Close()

	fmt.Printf("engine up @ %d Hz — playing %s\n", eng.SampleRate(), path)

	if err := eng.PlayFile(path); err != nil {
		fmt.Fprintln(os.Stderr, "play:", err)
		os.Exit(1)
	}

	// PlayFile is fire-and-forget on the audio thread. If main() returned now,
	// the deferred Close would tear the engine down before any sound came out.
	// Sleep to let it play. (A real engine will block on the transport instead.)
	time.Sleep(3 * time.Second)
}
