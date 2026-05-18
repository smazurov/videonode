package cmd

import (
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
	"github.com/spf13/cobra"
)

// CreateValidateEncodersCmd creates the validate-encoders command. The streamsConfigPath
// closure resolves the path at run time (after cobra has parsed flags), so callers can
// thread the same flag/env precedence the server uses.
func CreateValidateEncodersCmd(streamsConfigPath func(cmd *cobra.Command) string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-encoders",
		Short: "Validate hardware encoder availability",
		Long: `This command tests hardware encoders (H.264 and H.265) to determine which ones actually work ` +
			`on the current system. Results are saved to streams.toml.`,
		Run: func(cmd *cobra.Command, _ []string) {
			quiet, _ := cmd.Flags().GetBool("quiet")
			path := streamsConfigPath(cmd)
			streamStore := store.NewTOML(path)
			validationService := streams.NewValidationService(streamStore)
			encoders.RunValidateCommandWithOptions(validationService, quiet)
		},
	}

	cmd.Flags().BoolP("quiet", "q", false, "Suppress detailed validation progress output")
	cmd.Flags().String("streams-config", "", "Path to streams.toml (overrides STREAMS_CONFIG_FILE env)")
	return cmd
}
