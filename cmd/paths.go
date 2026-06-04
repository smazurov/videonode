package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// ResolveStreamsConfigPath picks the streams.toml path using the same precedence
// the server's humacli option pipeline uses for subcommands that short-circuit
// the heavy boot path: --streams-config flag → STREAMS_CONFIG_FILE env → default.
//
// Subcommands that need to read or write the persistent streams store should
// route through this helper so a CLI invocation never silently lands on a
// $PWD-relative phantom file.
func ResolveStreamsConfigPath(c *cobra.Command) string {
	if c != nil {
		if v, _ := c.Flags().GetString("streams-config"); v != "" {
			return v
		}
	}
	if v := os.Getenv("STREAMS_CONFIG_FILE"); v != "" {
		return v
	}
	return "streams.toml"
}
