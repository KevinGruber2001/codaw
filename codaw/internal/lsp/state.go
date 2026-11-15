package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// state is what the server remembers between messages: the files the editor has open, as they
// are in the editor, and which files it has reported problems for.
type state struct {
	mu        sync.Mutex
	documents map[string]*openDocument // by path: one file's URI can be spelled in several ways
	published map[string]bool
	handler   *protocol.Handler
	version   string
}

type openDocument struct {
	uri     string
	text    string
	version protocol.Integer
}

func newState(version string) *state {
	return &state{documents: map[string]*openDocument{}, published: map[string]bool{}, version: version}
}

func (s *state) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	capabilities := s.handler.CreateServerCapabilities()
	openClose := true
	change := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = protocol.TextDocumentSyncOptions{OpenClose: &openClose, Change: &change, Save: true}
	capabilities.CompletionProvider = &protocol.CompletionOptions{TriggerCharacters: []string{`"`, "=", "[", "."}}
	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo:   &protocol.InitializeResultServerInfo{Name: name, Version: &s.version},
	}, nil
}

func (s *state) didOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.update(ctx.Notify, params.TextDocument.URI, func(path string) {
		s.documents[path] = &openDocument{uri: params.TextDocument.URI, text: params.TextDocument.Text, version: params.TextDocument.Version}
	})
	return nil
}

func (s *state) didChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	s.update(ctx.Notify, params.TextDocument.URI, func(path string) {
		doc := s.documents[path]
		if doc == nil {
			doc = &openDocument{uri: params.TextDocument.URI}
			s.documents[path] = doc
		}
		doc.version = params.TextDocument.Version
		for _, change := range params.ContentChanges {
			if whole, ok := change.(protocol.TextDocumentContentChangeEventWhole); ok {
				doc.text = whole.Text
			}
		}
	})
	return nil
}

func (s *state) didSave(ctx *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.update(ctx.Notify, params.TextDocument.URI, func(string) {})
	return nil
}

func (s *state) didClose(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.update(ctx.Notify, params.TextDocument.URI, func(path string) { delete(s.documents, path) })
	return nil
}

// update changes what is open, then checks the project the file belongs to again.
func (s *state) update(notify glsp.NotifyFunc, uri string, change func(path string)) {
	path, err := uriToPath(uri)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	change(path)
	s.check(notify, path)
}

// check publishes the problems of every file of a project: a list for each, empty where there is
// nothing wrong, which is what clears a problem that was fixed.
func (s *state) check(notify glsp.NotifyFunc, path string) {
	projectFile, ok := findProjectFile(path)
	if !ok {
		s.publish(notify, path, nil)
		return
	}
	v := s.load(projectFile)
	for file := range v.docs {
		s.publish(notify, file, v.found[file])
	}
	if v.project == nil {
		// project.toml is being edited and does not parse: what was said about the other files stands.
		return
	}
	for file := range s.published {
		if v.docs[file] == nil && strings.HasPrefix(file, v.root+string(filepath.Separator)) {
			s.publish(notify, file, nil)
		}
	}
}

func (s *state) publish(notify glsp.NotifyFunc, path string, diagnostics []protocol.Diagnostic) {
	if len(diagnostics) == 0 {
		if !s.published[path] && s.documents[path] == nil {
			return
		}
		diagnostics = []protocol.Diagnostic{}
	}
	if len(diagnostics) > 0 {
		sort.SliceStable(diagnostics, func(i, j int) bool {
			a, b := diagnostics[i].Range.Start, diagnostics[j].Range.Start
			return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character
		})
		s.published[path] = true
	} else {
		delete(s.published, path)
	}
	params := protocol.PublishDiagnosticsParams{URI: pathToURI(path), Diagnostics: diagnostics}
	if doc := s.documents[path]; doc != nil {
		version := protocol.UInteger(max(doc.version, 0))
		params.URI, params.Version = doc.uri, &version
	}
	notify(protocol.ServerTextDocumentPublishDiagnostics, params)
}

// source is a file's text: the editor's if it is open, the disk's otherwise.
func (s *state) source(path string) (string, bool) {
	if doc := s.documents[path]; doc != nil {
		return doc.text, true
	}
	data, err := os.ReadFile(path)
	return string(data), err == nil
}
