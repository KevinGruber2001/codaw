package main

import (
	"encoding/json"
	"os"

	"github.com/KevinGruber2001/codaw/internal/schema"
	"github.com/spf13/cobra"
)

// schemaVersion changes when the shape of the output does, not when a key or an effect is added.
const schemaVersion = 1

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print what project files may contain, as JSON",
	Long: `Prints every section, key and effect the engine knows, with types, units, ranges
and defaults. For editors and tools: the same table the engine validates against,
so nothing about the file format has to be written down a second time.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := json.NewEncoder(os.Stdout)
		out.SetIndent("", "  ")
		return out.Encode(map[string]any{
			"version": schemaVersion,
			"files":   schema.Files,
			"effects": schema.Effects,
		})
	},
}
