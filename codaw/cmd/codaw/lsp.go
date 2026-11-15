package main

import (
	"github.com/KevinGruber2001/codaw/internal/lsp"
	"github.com/spf13/cobra"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run the language server for CodaW's project files, speaking LSP over stdio",
	Long: `Checks and explains CodaW's TOML files while they are edited: errors where
they are, completion, hover. Meant to be started by an editor (CodaW, VS Code, ...).

stdout carries only LSP messages; all logs go to stderr.`,
	Args: cobra.NoArgs,
	// As for serve: cobra's usage text must never land in the protocol stream.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return lsp.Run(version)
	},
}
