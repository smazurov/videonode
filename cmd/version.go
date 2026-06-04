package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/smazurov/videonode/internal/version"
)

// CreateVersionCmd creates the version command, which prints the embedded
// build metadata (set via ldflags by goreleaser). Defaults to a short
// human-readable line; pass --json for the full Info struct.
func CreateVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print build version and metadata",
		Run: func(cmd *cobra.Command, _ []string) {
			info := version.Get()
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(info); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode version info: %v\n", err)
					os.Exit(1)
				}
				return
			}
			fmt.Printf("videonode %s (%s, built %s, %s, %s)\n",
				info.Version, info.GitCommit, info.BuildDate, info.GoVersion, info.Platform)
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}
