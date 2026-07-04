package project

import (
	"fmt"
	"os"
	"path/filepath"
)

// Validate checks a fully loaded Project for logical errors.
// It runs after Load() — by this point all files are parsed,
// so we can check cross-file references (e.g. track.Bus referencing a bus ID).
//
// Returns a ValidationErrors slice so the caller sees ALL problems at once,
// not just the first one. This is much more useful when editing project files —
// you fix everything in one pass instead of playing whack-a-mole.
func Validate(p *Project) error {
	var errs ValidationErrors

	// We call each sub-validator and collect errors.
	// Each one appends to errs rather than returning early.
	errs = append(errs, validateTransport(p)...)
	errs = append(errs, validateMaster(p)...)
	errs = append(errs, validateBuses(p)...)
	errs = append(errs, validateTracks(p)...)
	errs = append(errs, validateReferences(p)...)

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// validateTransport checks the [transport] section.
func validateTransport(p *Project) ValidationErrors {
	var errs ValidationErrors
	t := p.Transport
	file := "project.toml"

	if t.BPM < 20 || t.BPM > 300 {
		errs = append(errs, &ValidationError{
			File:    file,
			Field:   "transport.bpm",
			Message: fmt.Sprintf("must be between 20 and 300, got %.1f", t.BPM),
		})
	}

	if t.TimeSig == "" {
		errs = append(errs, &ValidationError{
			File:    file,
			Field:   "transport.time_sig",
			Message: "required — e.g. \"4/4\"",
		})
	}

	validSampleRates := map[int]bool{44100: true, 48000: true, 88200: true, 96000: true}
	if !validSampleRates[t.SampleRate] {
		errs = append(errs, &ValidationError{
			File:    file,
			Field:   "transport.sample_rate",
			Message: fmt.Sprintf("must be 44100, 48000, 88200, or 96000 — got %d", t.SampleRate),
		})
	}

	validBitDepths := map[int]bool{16: true, 24: true, 32: true}
	if !validBitDepths[t.BitDepth] {
		errs = append(errs, &ValidationError{
			File:    file,
			Field:   "transport.bit_depth",
			Message: fmt.Sprintf("must be 16, 24, or 32 — got %d", t.BitDepth),
		})
	}

	return errs
}

// validateMaster checks the master settings.
func validateMaster(p *Project) ValidationErrors {
	var errs ValidationErrors

	if p.Master == nil {
		return errs
	}

	file := p.Master.SourceFile
	errs = append(errs, validateGain(file, "gain", p.Master.Gain)...)
	errs = append(errs, validateFXChain(file, "fx", p.Master.FX)...)

	return errs
}

// validateBuses checks each bus.
func validateBuses(p *Project) ValidationErrors {
	var errs ValidationErrors

	// Track seen IDs to catch duplicates.
	// Two buses with the same ID would cause silent bugs — tracks routing
	// to "drums" would be ambiguous about which bus they mean.
	seenIDs := make(map[string]string) // id → source file

	for _, b := range p.Buses {
		file := b.SourceFile

		if b.ID == "" {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   "id",
				Message: "required",
			})
			continue // skip further checks if no ID
		}

		if prev, exists := seenIDs[b.ID]; exists {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   "id",
				Message: fmt.Sprintf("duplicate bus ID %q — already defined in %s", b.ID, prev),
			})
		} else {
			seenIDs[b.ID] = file
		}

		errs = append(errs, validateGain(file, "gain", b.Gain)...)
		errs = append(errs, validateFXChain(file, "fx", b.FX)...)
	}

	return errs
}

// validateTracks checks each track and its clips.
func validateTracks(p *Project) ValidationErrors {
	var errs ValidationErrors

	seenIDs := make(map[string]string) // id → source file

	for _, t := range p.Tracks {
		file := t.SourceFile

		if t.ID == "" {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   "id",
				Message: "required",
			})
			continue
		}

		if prev, exists := seenIDs[t.ID]; exists {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   "id",
				Message: fmt.Sprintf("duplicate track ID %q — already defined in %s", t.ID, prev),
			})
		} else {
			seenIDs[t.ID] = file
		}

		errs = append(errs, validateGain(file, "gain", t.Gain)...)
		errs = append(errs, validatePan(file, "pan", t.Pan)...)
		errs = append(errs, validateFXChain(file, "fx", t.FX)...)
		errs = append(errs, validateClips(t)...)
		errs = append(errs, validateAutomation(t)...)
	}

	return errs
}

// validateAutomation checks each automation lane on a track: a target must be
// set and there must be at least one point with a non-negative beat. The exact
// target syntax (gain/pan/fx[N].key) is parsed by the engine, which logs and
// skips anything it doesn't recognise, so we keep validation light here.
func validateAutomation(t *Track) ValidationErrors {
	var errs ValidationErrors
	file := t.SourceFile

	for i, a := range t.Automation {
		field := func(f string) string {
			return fmt.Sprintf("automation[%d].%s", i, f)
		}

		if a.Target == "" {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   field("target"),
				Message: "required",
			})
		}

		if len(a.Points) == 0 {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   field("point"),
				Message: "at least one point is required",
			})
		}

		for j, pt := range a.Points {
			if pt.Beat < 0 {
				errs = append(errs, &ValidationError{
					File:    file,
					Field:   fmt.Sprintf("automation[%d].point[%d].beat", i, j),
					Message: fmt.Sprintf("must be >= 0, got %.2f", pt.Beat),
				})
			}
		}
	}

	return errs
}

// validateClips checks each clip on a track.
func validateClips(t *Track) ValidationErrors {
	var errs ValidationErrors
	file := t.SourceFile

	for i, c := range t.Clips {
		field := func(f string) string {
			// Helper to build field paths like "clip[0].end"
			return fmt.Sprintf("clip[%d].%s", i, f)
		}

		if c.File == "" {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   field("file"),
				Message: "required",
			})
		} else {
			// Check the audio file actually exists on disk.
			// We resolve relative to the project root (stored in RootDir on the track
			// via SourceFile's directory — we use the track's source file dir here
			// since clips are relative to the project root).
			// Note: in phase 3, the engine will also check this — but catching it
			// here gives a much clearer error message.
			clipPath := filepath.Join(filepath.Dir(file), "..", c.File)
			if _, err := os.Stat(clipPath); os.IsNotExist(err) {
				// We warn but don't hard-fail — the file might be generated later
				// or the path might be intentional (e.g. a placeholder).
				errs = append(errs, &ValidationError{
					File:    file,
					Field:   field("file"),
					Message: fmt.Sprintf("audio file not found: %s", c.File),
				})
			}
		}

		if c.Start < 0 {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   field("start"),
				Message: fmt.Sprintf("must be >= 0, got %.1f", c.Start),
			})
		}

		if c.End <= c.Start {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   field("end"),
				Message: fmt.Sprintf("must be greater than start (%.1f), got %.1f", c.Start, c.End),
			})
		}

		if c.Offset < 0 {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   field("offset"),
				Message: fmt.Sprintf("must be >= 0, got %.2f", c.Offset),
			})
		}

		errs = append(errs, validateGain(file, field("gain"), c.Gain)...)
	}

	return errs
}

// validateReferences checks cross-file consistency.
// This runs after all individual files are validated, so we have the full picture.
func validateReferences(p *Project) ValidationErrors {
	var errs ValidationErrors

	// Build a set of valid bus IDs for O(1) lookup.
	busIDs := make(map[string]bool)
	for _, b := range p.Buses {
		busIDs[b.ID] = true
	}

	// Check every track that specifies a bus actually references a real one.
	for _, t := range p.Tracks {
		if t.Bus != "" && !busIDs[t.Bus] {
			errs = append(errs, &ValidationError{
				File:    t.SourceFile,
				Field:   "bus",
				Message: fmt.Sprintf("references bus %q which does not exist", t.Bus),
			})
		}
	}

	return errs
}

// ─────────────────────────────────────────────
//  Shared helpers
// ─────────────────────────────────────────────

// validateGain checks a dB gain value is within the allowed range.
// Extracted as a helper because gain appears on tracks, buses, master, and clips.
func validateGain(file, field string, gain float64) ValidationErrors {
	if gain < -60 || gain > 6 {
		return ValidationErrors{&ValidationError{
			File:    file,
			Field:   field,
			Message: fmt.Sprintf("gain must be between -60 and +6 dB, got %.1f", gain),
		}}
	}
	return nil
}

// validatePan checks a pan value is within [-1, 1].
func validatePan(file, field string, pan float64) ValidationErrors {
	if pan < -1 || pan > 1 {
		return ValidationErrors{&ValidationError{
			File:    file,
			Field:   field,
			Message: fmt.Sprintf("pan must be between -1.0 and 1.0, got %.2f", pan),
		}}
	}
	return nil
}

// validateFXChain checks each effect in a chain has a type.
// Plugin-specific param validation happens in the engine when it loads the plugin.
func validateFXChain(file, field string, fxs []FX) ValidationErrors {
	var errs ValidationErrors

	for i, fx := range fxs {
		if fx.Type == "" {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   fmt.Sprintf("%s[%d].type", field, i),
				Message: "required — e.g. \"reverb\", \"eq_3band\", \"compressor\"",
			})
		}
	}

	return errs
}
