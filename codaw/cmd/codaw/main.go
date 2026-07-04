package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/KevinGruber2001/codaw/internal/engine"
	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/KevinGruber2001/codaw/internal/state"
	"github.com/KevinGruber2001/codaw/internal/watcher"
)

// main is the entrypoint. It just runs the root cobra command.
// All logic lives in the subcommands — main stays tiny.
func main() {
	if err := rootCmd.Execute(); err != nil {
		// cobra already prints the error, we just need to exit non-zero
		os.Exit(1)
	}
}

// version is stamped in at build time via -ldflags "-X main.version=X.Y.Z"
// (see .github/workflows/release.yml). Defaults to "dev" for local builds.
var version = "dev"

// rootCmd is the base command — running `codaw` with no subcommand shows help.
// Setting Version enables the built-in `codaw --version` flag.
var rootCmd = &cobra.Command{
	Use:     "codaw",
	Version: version,
	Short:   "CodaW — a code-first digital audio workstation",
	Long: `CodaW is a DAW where your project lives in plain text files.
Define tracks, clips, effects, and automation in TOML —
then git commit your whole session like a software project.`,
}

// init registers all subcommands on the root command.
// Go calls init() automatically before main().
// We use init() here (instead of doing it in main) because it keeps
// each command self-contained — if we later split commands into
// separate files, each file registers its own command in its own init().
func init() {
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(playCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(renderCmd)
}

// ─────────────────────────────────────────────
//  validate command
// ─────────────────────────────────────────────

var validateCmd = &cobra.Command{
	Use:   "validate <project.toml>",
	Short: "Validate a project file and all referenced files",
	Long: `Loads a project.toml and all tracks, buses, and master files it references.
Reports any structural errors, invalid values, or broken references.

Example:
  codaw validate ~/music/mytrack/project.toml`,

	// Args: cobra.ExactArgs(1) means this command requires exactly one argument.
	// Cobra will print a helpful error if the user provides 0 or 2+ arguments.
	Args: cobra.ExactArgs(1),

	// RunE is like Run but returns an error.
	// Cobra will print the error and exit non-zero if RunE returns an error.
	// Always use RunE over Run so errors propagate correctly.
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		fmt.Printf("loading %s\n", path)

		// Phase 1: load the project
		p, err := project.Load(path)
		if err != nil {
			return fmt.Errorf("load failed: %w", err)
		}

		fmt.Printf("  ✓ parsed project.toml\n")
		fmt.Printf("  ✓ loaded master (%s)\n", p.Layout.Master)
		fmt.Printf("  ✓ loaded %d bus(es)\n", len(p.Buses))
		fmt.Printf("  ✓ loaded %d track(s)\n", len(p.Tracks))

		// Phase 1: validate the project
		if err := project.Validate(p); err != nil {
			// Print each validation error on its own line
			fmt.Fprintf(os.Stderr, "\nvalidation failed:\n%v\n", err)
			os.Exit(1)
		}

		// Print a summary of what was loaded
		fmt.Printf("\nproject: %s\n", p.Meta.Name)
		fmt.Printf("  bpm:         %.0f\n", p.Transport.BPM)
		fmt.Printf("  time sig:    %s\n", p.Transport.TimeSig)
		fmt.Printf("  sample rate: %d Hz\n", p.Transport.SampleRate)
		fmt.Printf("  bit depth:   %d bit\n", p.Transport.BitDepth)

		if len(p.Buses) > 0 {
			fmt.Printf("\nbuses:\n")
			for _, b := range p.Buses {
				fmt.Printf("  • %s (gain: %.1f dB, fx: %d)\n", b.ID, b.Gain, len(b.FX))
			}
		}

		fmt.Printf("\ntracks:\n")
		for _, t := range p.Tracks {
			busLabel := "→ master"
			if t.Bus != "" {
				busLabel = fmt.Sprintf("→ %s", t.Bus)
			}
			fmt.Printf("  • %-12s %s  (gain: %.1f dB, clips: %d, fx: %d)\n",
				t.ID, busLabel, t.Gain, len(t.Clips), len(t.FX))
		}

		fmt.Printf("\n✓ project is valid\n")
		return nil
	},
}

// ─────────────────────────────────────────────
//  play / watch commands
// ─────────────────────────────────────────────

var playCmd = &cobra.Command{
	Use:   "play <project.toml>",
	Short: "Load a project and play it through the speakers",
	Long: `Loads and validates a project, builds the audio graph, and plays it.
Runs until you press Ctrl-C.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEngine(args[0], false)
	},
}

var watchCmd = &cobra.Command{
	Use:   "watch <project.toml>",
	Short: "Play a project and hot-reload it as you edit the TOML files",
	Long: `Like 'play', but also watches the project files. Editing a track's
gain, pan, or mute/solo and saving updates the running audio live — no restart.
Runs until you press Ctrl-C.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEngine(args[0], true)
	},
}

var renderCmd = &cobra.Command{
	Use:   "render <project.toml> <out.wav>",
	Short: "Render a project offline to a WAV file",
	Long: `Loads and validates a project, then renders it to a 16-bit WAV file
without playing through the speakers. Faster than real time.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, out := args[0], args[1]

		p, err := project.Load(path)
		if err != nil {
			return fmt.Errorf("load failed: %w", err)
		}
		if err := project.Validate(p); err != nil {
			return fmt.Errorf("invalid project:\n%v", err)
		}

		store := state.New(p)
		eng, err := engine.NewOffline(store)
		if err != nil {
			return fmt.Errorf("offline engine init failed: %w", err)
		}
		defer eng.Close()

		if err := eng.Load(); err != nil {
			return fmt.Errorf("build graph failed: %w", err)
		}

		fmt.Printf("rendering %q → %s\n", p.Meta.Name, out)
		// 2s tail so reverb/echo can ring out instead of cutting abruptly.
		if err := eng.Render(out, 2.0); err != nil {
			return fmt.Errorf("render failed: %w", err)
		}
		fmt.Println("✓ done")
		return nil
	},
}

// runEngine is the shared playback driver for 'play' and 'watch'.
// The only difference is whether the file watcher is started.
func runEngine(path string, watch bool) error {
	// 1. Load + validate up front — refuse to play a broken project.
	p, err := project.Load(path)
	if err != nil {
		return fmt.Errorf("load failed: %w", err)
	}
	if err := project.Validate(p); err != nil {
		return fmt.Errorf("invalid project:\n%v", err)
	}

	// 2. The Store is the single source of runtime truth. The engine reads from
	//    it and (in watch mode) the watcher writes mutations into it.
	store := state.New(p)

	// 3. Boot the audio engine and build the graph.
	eng, err := engine.New(store)
	if err != nil {
		return fmt.Errorf("engine init failed: %w", err)
	}
	defer eng.Close()

	if err := eng.Load(); err != nil {
		return fmt.Errorf("build graph failed: %w", err)
	}
	if err := eng.Play(); err != nil {
		return fmt.Errorf("playback failed: %w", err)
	}

	fmt.Printf("▶ playing %q — %.0f BPM, %d track(s)\n", p.Meta.Name, p.Transport.BPM, len(p.Tracks))

	// 4. In watch mode, start the file watcher. It diffs TOML changes and pushes
	//    mutations into the Store; the engine is subscribed and reacts live.
	if watch {
		w, err := watcher.New(store)
		if err != nil {
			return fmt.Errorf("watcher init failed: %w", err)
		}
		if err := w.Start(); err != nil {
			return fmt.Errorf("watcher start failed: %w", err)
		}
		defer w.Stop()
		fmt.Println("👀 watching for edits — try changing a gain value and saving")
	}

	// 5. Block until Ctrl-C (SIGINT) or SIGTERM, then let the defers clean up.
	fmt.Println("press Ctrl-C to stop")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("\nstopping…")
	return nil
}
