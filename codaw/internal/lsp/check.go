package lsp

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/KevinGruber2001/codaw/internal/project"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// check validates the project a document belongs to, with that document's text from the editor and
// every other file from disk, and publishes the result for every file of the project: errors where
// there are some, an empty list where there are none (which clears what was shown before).
func (s *state) check(ctx *glsp.Context, uri string, text string) {
	path, err := uriToPath(uri)
	if err != nil {
		return
	}
	projectFile, ok := findProjectFile(path)
	if !ok {
		return // not part of a CodaW project
	}

	found := map[string][]protocol.Diagnostic{} // absolute path -> its diagnostics
	texts := map[string]string{path: text}      // the text positions are looked up in

	p, err := project.Load(projectFile)
	if err != nil {
		// A file on disk does not load: report it on that file, at the parse error if there is one.
		file := projectFile
		var loadErr *project.LoadError
		if errors.As(err, &loadErr) {
			file = loadErr.File
		}
		found[file] = append(found[file], diagnosticForError(err))
		s.publish(ctx, []string{projectFile, path}, found)
		return
	}

	if err := replaceWithText(p, path, text); err != nil {
		// The text being typed does not parse: that is the one error worth showing right now.
		found[path] = append(found[path], diagnosticForError(err))
	} else if err := project.Validate(p); err != nil {
		var problems project.ValidationErrors
		if errors.As(err, &problems) {
			for _, problem := range problems {
				fileText, ok := texts[problem.File]
				if !ok {
					data, _ := os.ReadFile(problem.File)
					fileText = string(data)
					texts[problem.File] = fileText
				}
				found[problem.File] = append(found[problem.File], diagnostic(fieldRange(fileText, problem.Field), problem.Message))
			}
		}
	}
	s.publish(ctx, projectFiles(p, projectFile), found)
}

// replaceWithText puts the editor's text of one file in place of what was loaded from disk. A file
// that is not a track, bus or master (project.toml itself) is checked as it is on disk.
func replaceWithText(p *project.Project, path, text string) error {
	if p.Master != nil && p.Master.SourceFile == path {
		var master project.Master
		if _, err := toml.Decode(text, &master); err != nil {
			return err
		}
		master.SourceFile = path
		p.Master = &master
		return nil
	}
	for i, bus := range p.Buses {
		if bus.SourceFile == path {
			var updated project.Bus
			if _, err := toml.Decode(text, &updated); err != nil {
				return err
			}
			updated.SourceFile = path
			p.Buses[i] = &updated
			return nil
		}
	}
	for i, track := range p.Tracks {
		if track.SourceFile == path {
			var updated project.Track
			if _, err := toml.Decode(text, &updated); err != nil {
				return err
			}
			updated.SourceFile = path
			p.Tracks[i] = &updated
			return nil
		}
	}
	return nil
}

// publish sends the diagnostics of every given file, and of every file that has some.
func (s *state) publish(ctx *glsp.Context, files []string, found map[string][]protocol.Diagnostic) {
	all := map[string]bool{}
	for _, file := range files {
		all[file] = true
	}
	for file := range found {
		all[file] = true
	}
	for file := range all {
		diagnostics := found[file]
		if diagnostics == nil {
			diagnostics = []protocol.Diagnostic{} // an empty list, not null: it clears the file
		}
		ctx.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
			URI:         pathToURI(file),
			Diagnostics: diagnostics,
		})
	}
}

func projectFiles(p *project.Project, projectFile string) []string {
	files := []string{projectFile}
	if p.Master != nil {
		files = append(files, p.Master.SourceFile)
	}
	for _, bus := range p.Buses {
		files = append(files, bus.SourceFile)
	}
	for _, track := range p.Tracks {
		files = append(files, track.SourceFile)
	}
	return files
}

// findProjectFile walks up from a file to the project.toml it belongs to.
func findProjectFile(path string) (string, bool) {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "project.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", false
		}
	}
}

func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	return filepath.Clean(u.Path), nil
}

func pathToURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
