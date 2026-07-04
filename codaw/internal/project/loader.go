package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Load reads a project.toml file and all files it references,
// returning a fully assembled Project ready for validation or use.
//
// path is the path to project.toml — it can be absolute or relative.
//
// The general flow is:
//  1. Read + decode project.toml into a Project struct (meta, transport, layout)
//  2. Resolve the root directory (so relative paths work)
//  3. Load master.toml
//  4. Load each bus file
//  5. Load each track file
//  6. Return the assembled Project
func Load(path string) (*Project, error) {
	// filepath.Abs converts a relative path like "./testdata/basic/project.toml"
	// to an absolute path like "/home/kevin/codaw/testdata/basic/project.toml".
	// This is important because we later derive the RootDir from it,
	// and all other paths are resolved relative to RootDir.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, &LoadError{File: path, Err: fmt.Errorf("could not resolve path: %w", err)}
	}

	// Check the file exists before trying to decode it.
	// This gives a cleaner error than the one toml.DecodeFile would produce.
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, &LoadError{File: absPath, Err: fmt.Errorf("file not found")}
	}

	// Decode project.toml into a Project struct.
	// toml.DecodeFile reads the file and maps TOML keys to struct fields
	// using the `toml:"..."` tags we defined in project.go.
	// The second return value (toml.MetaData) tells us which keys were
	// actually present in the file — useful for detecting unknown keys,
	// but we're not using it in phase 1.
	var p Project
	if _, err := toml.DecodeFile(absPath, &p); err != nil {
		return nil, &LoadError{File: absPath, Err: err}
	}

	// RootDir is the directory containing project.toml.
	// filepath.Dir("/home/kevin/music/mytrack/project.toml")
	// returns "/home/kevin/music/mytrack"
	p.RootDir = filepath.Dir(absPath)

	// Now load each referenced file, using p.RootDir to resolve paths.
	if err := loadMaster(&p); err != nil {
		return nil, err
	}

	if err := loadBuses(&p); err != nil {
		return nil, err
	}

	if err := loadTracks(&p); err != nil {
		return nil, err
	}

	return &p, nil
}

// resolve turns a path that is relative to the project root into an absolute path.
// For example, "tracks/kick.toml" becomes "/home/kevin/music/mytrack/tracks/kick.toml".
//
// This is a helper used by all the load* functions below.
// Having it as a named function makes the intent clear and avoids
// repeating filepath.Join(p.RootDir, ...) everywhere.
func resolve(p *Project, relPath string) string {
	return filepath.Join(p.RootDir, relPath)
}

// loadMaster loads the master.toml file referenced in p.Layout.Master.
func loadMaster(p *Project) error {
	if p.Layout.Master == "" {
		// Master is optional for now — if not specified, use defaults.
		p.Master = &Master{Gain: 0.0, Limiter: false}
		return nil
	}

	path := resolve(p, p.Layout.Master)

	var m Master
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return &LoadError{File: path, Err: err}
	}

	// Store the source file path so error messages and the file watcher
	// know where this data came from.
	m.SourceFile = path
	p.Master = &m
	return nil
}

// loadBuses loads each bus file listed in p.Layout.Buses.
func loadBuses(p *Project) error {
	// Pre-allocate the slice to the exact size we need.
	// This is a minor Go performance habit — avoids repeated allocations
	// as the slice grows.
	p.Buses = make([]*Bus, 0, len(p.Layout.Buses))

	for _, relPath := range p.Layout.Buses {
		path := resolve(p, relPath)

		var b Bus
		if _, err := toml.DecodeFile(path, &b); err != nil {
			return &LoadError{File: path, Err: err}
		}

		b.SourceFile = path
		p.Buses = append(p.Buses, &b)
	}

	return nil
}

// loadTracks loads each track file listed in p.Layout.Tracks.
func loadTracks(p *Project) error {
	p.Tracks = make([]*Track, 0, len(p.Layout.Tracks))

	for _, relPath := range p.Layout.Tracks {
		path := resolve(p, relPath)

		var t Track
		if _, err := toml.DecodeFile(path, &t); err != nil {
			return &LoadError{File: path, Err: err}
		}

		t.SourceFile = path
		p.Tracks = append(p.Tracks, &t)
	}

	return nil
}
