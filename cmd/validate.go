package cmd

import (
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/store"
	"github.com/spf13/cobra"
)

// CreateValidateEncodersCmd creates the validate-encoders command
func CreateValidateEncodersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-encoders",
		Short: "Validate hardware encoder availability",
		Long:  `This command tests hardware encoders (H.264 and H.265) to determine which ones actually work on the current system. Results are saved to streams.toml.`,
		Run: func(cmd *cobra.Command, args []string) {
			quiet, _ := cmd.Flags().GetBool("quiet")
			// Create validation service for encoder validation
			streamStore := store.NewTOML("streams.toml")
			validationService := streams.NewValidationService(streamStore)
			encoders.RunValidateCommandWithOptions(validationService, quiet)
		},
	}

	cmd.Flags().BoolP("quiet", "q", false, "Suppress detailed validation progress output")
	return cmd
}
