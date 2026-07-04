package project

import "fmt"

// ─────────────────────────────────────────────
//  LoadError
// ─────────────────────────────────────────────

// LoadError is returned when a file cannot be read or parsed.
// It wraps the underlying error with context about which file failed.
//
// We define our own error types (rather than using fmt.Errorf) because:
// 1. Callers can type-assert to get structured info (file, line, etc.)
// 2. The UI/LSP can later show errors in the right file at the right position
// 3. Tests can assert on specific error types, not string matching
type LoadError struct {
	// File is the path to the TOML file that failed to load.
	File string

	// Err is the underlying error (e.g. from os.ReadFile or toml.Decode).
	Err error
}

// Error implements the error interface.
// Go convention: error messages are lowercase, no trailing period.
func (e *LoadError) Error() string {
	return fmt.Sprintf("failed to load %s: %v", e.File, e.Err)
}

// Unwrap allows errors.Is and errors.As to unwrap through LoadError.
// This is Go 1.13+ error wrapping — lets callers do:
//
//	errors.Is(err, os.ErrNotExist)
//
// even when the error is wrapped in a LoadError.
func (e *LoadError) Unwrap() error {
	return e.Err
}

// ─────────────────────────────────────────────
//  ValidationError
// ─────────────────────────────────────────────

// ValidationError is returned when a project file is valid TOML
// but contains invalid values (e.g. gain out of range, missing required field).
type ValidationError struct {
	// File is the TOML file containing the invalid value.
	File string

	// Field is the dot-path to the field (e.g. "tracks[0].clip[1].end").
	Field string

	// Message describes what's wrong in plain English.
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s — %s", e.File, e.Field, e.Message)
}

// ─────────────────────────────────────────────
//  ValidationErrors (multiple)
// ─────────────────────────────────────────────

// ValidationErrors is a slice of ValidationError.
// We use a named type so we can implement the error interface on it —
// this lets the validator return all errors at once instead of stopping
// at the first one, which is much more useful when editing TOML files.
type ValidationErrors []*ValidationError

func (ve ValidationErrors) Error() string {
	if len(ve) == 1 {
		return ve[0].Error()
	}
	msg := fmt.Sprintf("%d validation errors:\n", len(ve))
	for _, e := range ve {
		msg += fmt.Sprintf("  • %s\n", e.Error())
	}
	return msg
}

// ─────────────────────────────────────────────
//  ReferenceError
// ─────────────────────────────────────────────

// ReferenceError is returned when something references an ID that doesn't exist.
// For example: a track with bus = "drums" but no bus with id = "drums" exists.
// This is a separate error type from ValidationError because it requires
// cross-file analysis — you can't catch it by validating one file in isolation.
type ReferenceError struct {
	// File is the file containing the bad reference.
	File string

	// Field is the field containing the reference (e.g. "bus").
	Field string

	// Value is the referenced ID that couldn't be found (e.g. "drums").
	Value string

	// Kind describes what kind of thing wasn't found (e.g. "bus", "track").
	Kind string
}

func (e *ReferenceError) Error() string {
	return fmt.Sprintf("%s: %s references %s %q which does not exist",
		e.File, e.Field, e.Kind, e.Value)
}
