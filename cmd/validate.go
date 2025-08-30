package cmd

import (
	"github.com/smazurov/videonode/internal/config"
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/spf13/cobra"
)

// CreateValidateEncodersCmd creates the validate-encoders command with StreamManager
func CreateValidateEncodersCmd(streamManager *config.StreamManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-encoders",
		Short: "Validate hardware encoder availability",
		Long:  `This command tests hardware encoders (H.264 and H.265) to determine which ones actually work on the current system. Results are saved to streams.toml.`,
		Run: func(cmd *cobra.Command, args []string) {
			quiet, _ := cmd.Flags().GetBool("quiet")
			// Use the provided StreamManager
			encoders.RunValidateCommandWithOptions(streamManager, quiet)
		},
	}

	cmd.Flags().BoolP("quiet", "q", false, "Suppress detailed validation progress output")
	return cmd
}
