// Package lsp is CodaW's language server: it checks and explains the project's TOML files as
// they are edited. It reads text and never touches audio; it runs as `codaw lsp`.
package lsp

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
)

const name = "codaw"

// Run serves LSP on stdin/stdout until the editor shuts the server down.
func Run(version string) error {
	s := &state{documents: map[string]string{}, version: version}
	handler := protocol.Handler{
		Initialize:             s.initialize,
		Initialized:            func(*glsp.Context, *protocol.InitializedParams) error { return nil },
		Shutdown:               func(*glsp.Context) error { return nil },
		TextDocumentDidOpen:    s.didOpen,
		TextDocumentDidChange:  s.didChange,
		TextDocumentDidClose:   s.didClose,
		TextDocumentCompletion: s.completion,
		TextDocumentHover:      s.hover,
	}
	// initialize derives the server's capabilities from the handlers that are set.
	s.handler = &handler
	return server.NewServer(&handler, name, false).RunStdio()
}
