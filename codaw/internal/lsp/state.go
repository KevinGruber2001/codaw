package lsp

import (
	"sync"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// state is what the server remembers between messages: for now, the text of every file
// the editor has open, by URI. Diagnostics, completion and hover all work on this text,
// not on the disk, so they see unsaved changes.
type state struct {
	mu        sync.Mutex
	documents map[string]string // file path -> current text (by path: URIs of one file can be spelled differently)
	handler   *protocol.Handler // to derive the capabilities from in initialize
	version   string            // reported to the editor in initialize
}

func (s *state) initialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	capabilities := s.handler.CreateServerCapabilities()
	// Full sync: on every change the editor sends the whole text, not a diff. Simpler,
	// and these files are a few dozen lines.
	openClose := true
	change := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = protocol.TextDocumentSyncOptions{OpenClose: &openClose, Change: &change}
	// Suggest without being asked after these: a quote opens a value, "=" starts one, "[" a header.
	capabilities.CompletionProvider = &protocol.CompletionOptions{TriggerCharacters: []string{`"`, "=", "[", "."}}
	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo:   &protocol.InitializeResultServerInfo{Name: name, Version: &s.version},
	}, nil
}

func (s *state) didOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	s.documents[path] = params.TextDocument.Text
	s.mu.Unlock()
	s.check(ctx, params.TextDocument.URI, params.TextDocument.Text)
	return nil
}

func (s *state) didChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	var text string
	for _, change := range params.ContentChanges {
		if whole, ok := change.(protocol.TextDocumentContentChangeEventWhole); ok {
			text = whole.Text
		}
	}
	s.documents[path] = text
	s.mu.Unlock()
	s.check(ctx, params.TextDocument.URI, text)
	return nil
}

func (s *state) didClose(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.documents, path)
	return nil
}
