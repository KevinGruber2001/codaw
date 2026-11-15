package project

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/KevinGruber2001/codaw/internal/schema"
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

	errs = append(errs, checkNumbers(file, "transport.", schema.ProjectFile, "transport", t)...)

	if t.TimeSig == "" {
		errs = append(errs, &ValidationError{
			File:    file,
			Field:   "transport.time_sig",
			Message: "required — e.g. \"4/4\"",
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
	errs = append(errs, checkNumbers(file, "", schema.MasterFile, "", *p.Master)...)
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

		errs = append(errs, checkNumbers(file, "", schema.BusFile, "", *b)...)
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

		errs = append(errs, checkNumbers(file, "", schema.TrackFile, "", *t)...)
		errs = append(errs, validateFXChain(file, "fx", t.FX)...)
		errs = append(errs, validateClips(p.RootDir, t)...)
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
			errs = append(errs, checkNumbers(file, fmt.Sprintf("automation[%d].point[%d].", i, j), schema.TrackFile, "automation.point", pt)...)
		}
	}

	return errs
}

// validateClips checks each clip on a track.
func validateClips(root string, t *Track) ValidationErrors {
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
			// Clip files are relative to the project's folder, wherever the track file is: the same
			// place the engine looks.
			clipPath := filepath.Join(root, filepath.FromSlash(c.File))
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

		if c.End <= c.Start {
			errs = append(errs, &ValidationError{
				File:    file,
				Field:   field("end"),
				Message: fmt.Sprintf("must be greater than start (%.1f), got %.1f", c.Start, c.End),
			})
		}

		errs = append(errs, checkNumbers(file, field(""), schema.TrackFile, "clip", c)...)
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

// checkNumbers checks every number of a struct against its key in the schema: the range, or the
// few values allowed. The struct's fields are matched to keys by their toml tag, so a field added
// to both is checked without anything written here.
func checkNumbers(file, prefix string, kind schema.FileKind, section string, value any) ValidationErrors {
	var errs ValidationErrors
	f, _ := schema.FileOf(kind)
	keys, _ := f.Section(section)
	v := reflect.ValueOf(value)
	for i := 0; i < v.NumField(); i++ {
		key, ok := keys.Key(v.Type().Field(i).Tag.Get("toml"))
		if !ok || key.Kind != schema.Number {
			continue
		}
		var number float64
		switch field := v.Field(i); {
		case field.CanFloat():
			number = field.Float()
		case field.CanInt():
			number = float64(field.Int())
		default:
			continue
		}
		if problem := key.Check(number); problem != "" {
			errs = append(errs, &ValidationError{File: file, Field: prefix + key.Name, Message: problem})
		}
	}
	return errs
}

// validateFXChain checks each effect in a chain has a type, and that the parameters of the
// effects the engine knows are in range.
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

		effect, known := schema.EffectNamed(fx.Type)
		if !known {
			continue // the engine skips an effect it does not know
		}
		for _, param := range effect.Params {
			if number, ok := schema.AsNumber(fx.Params[param.Name]); ok {
				if problem := param.Check(number); problem != "" {
					errs = append(errs, &ValidationError{
						File:    file,
						Field:   fmt.Sprintf("%s[%d].%s", field, i, param.Name),
						Message: problem,
					})
				}
			}
		}
	}

	return errs
}
