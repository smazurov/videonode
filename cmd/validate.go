package cmd

import (
	"github.com/smazurov/videonode/internal/encoders"
	"github.com/spf13/cobra"
)

// ValidateEncodersCmd represents the validate-encoders command
// The output file will be set via SetValidationOutput before running
var ValidateEncodersCmd = &cobra.Command{
	Use:   "validate-encoders",
	Short: "Validate hardware encoder availability",
	Long:  `This command tests hardware encoders (H.264 and H.265) to determine which ones actually work on the current system.`,
	Run: func(cmd *cobra.Command, args []string) {
		outputFile, _ := cmd.Flags().GetString("output")
		quiet, _ := cmd.Flags().GetBool("quiet")
		encoders.RunValidateCommandWithOptions(outputFile, quiet)
	},
}

// SetValidationOutput sets the default output file for the validation command
func SetValidationOutput(outputFile string) {
	ValidateEncodersCmd.Flags().Set("output", outputFile)
}

func init() {
	ValidateEncodersCmd.Flags().StringP("output", "o", "validated_encoders.toml", "Output file for validation results")
	ValidateEncodersCmd.Flags().BoolP("quiet", "q", false, "Suppress detailed validation progress output")
}
