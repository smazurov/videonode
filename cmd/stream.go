package cmd

import (
	"github.com/spf13/cobra"
)

// CreateStreamCmd creates the `stream` parent command with list/get/create/delete
// subcommands. Each subcommand hits the corresponding /api/streams endpoint on
// the running daemon.
func CreateStreamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stream",
		Short: "Manage streams (encoder + audio + publish targets)",
		Long: `CRUD over /api/streams on the running daemon. Each stream is an encoder ` +
			`identity (stream-id = encoder identity) referencing an upstream source or composer.`,
	}
	addAPIFlags(cmd)
	cmd.AddCommand(
		newCRUDListCmd("stream", "/api/streams"),
		newCRUDGetCmd("stream", "/api/streams", "stream_id"),
		newCRUDCreateCmd("stream", "/api/streams"),
		newCRUDDeleteCmd("stream", "/api/streams", "stream_id"),
	)
	return cmd
}

// CreateSourceCmd creates the `source` parent command with list/get/create/delete
// subcommands hitting /api/sources.
func CreateSourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage frame producers (devices, test-pattern)",
		Long: `CRUD over /api/sources on the running daemon. A source is one frame ` +
			`producer (V4L2 device or RPC-driven test pattern) referenced by composers and streams.`,
	}
	addAPIFlags(cmd)
	cmd.AddCommand(
		newCRUDListCmd("source", "/api/sources"),
		newCRUDGetCmd("source", "/api/sources", "source_id"),
		newCRUDCreateCmd("source", "/api/sources"),
		newCRUDDeleteCmd("source", "/api/sources", "source_id"),
	)
	return cmd
}

// CreateComposerCmd creates the `composer` parent command with list/get/create/delete
// subcommands hitting /api/composers.
func CreateComposerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "composer",
		Short: "Manage GPU composers (canvas + inputs + layout)",
		Long: `CRUD over /api/composers on the running daemon. A composer aggregates ` +
			`N sources onto a BGRA canvas and broadcasts the result for downstream encoders.`,
	}
	addAPIFlags(cmd)
	cmd.AddCommand(
		newCRUDListCmd("composer", "/api/composers"),
		newCRUDGetCmd("composer", "/api/composers", "composer_id"),
		newCRUDCreateCmd("composer", "/api/composers"),
		newCRUDDeleteCmd("composer", "/api/composers", "composer_id"),
	)
	return cmd
}

// newCRUDListCmd builds a "list" subcommand that GETs the collection endpoint.
func newCRUDListCmd(entity, path string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all " + entity + "s",
		RunE: func(c *cobra.Command, _ []string) error {
			client := newAPIClient(c)
			var out any
			if err := client.do("GET", path, nil, &out); err != nil {
				return err
			}
			return printJSON(c, out)
		},
	}
}

// newCRUDGetCmd builds a "get <id>" subcommand that GETs one item.
func newCRUDGetCmd(entity, path, idLabel string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <" + idLabel + ">",
		Short: "Get one " + entity + " by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client := newAPIClient(c)
			var out any
			if err := client.do("GET", path+"/"+args[0], nil, &out); err != nil {
				return err
			}
			return printJSON(c, out)
		},
	}
}

// newCRUDCreateCmd builds a "create" subcommand that POSTs a JSON body. The body
// is read from --file or stdin; the CLI is intentionally a thin pass-through so
// the daemon's API model owns the schema.
func newCRUDCreateCmd(entity, path string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a " + entity + " from a JSON body (--file or stdin)",
		RunE: func(c *cobra.Command, _ []string) error {
			client := newAPIClient(c)
			body, err := readJSONBody(c)
			if err != nil {
				return err
			}
			var out any
			if err := client.do("POST", path, body, &out); err != nil {
				return err
			}
			return printJSON(c, out)
		},
	}
	cmd.Flags().StringP("file", "f", "", "Path to JSON body (omit or \"-\" to read stdin)")
	return cmd
}

// newCRUDDeleteCmd builds a "delete <id>" subcommand.
func newCRUDDeleteCmd(entity, path, idLabel string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <" + idLabel + ">",
		Short: "Delete a " + entity + " by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client := newAPIClient(c)
			return client.do("DELETE", path+"/"+args[0], nil, nil)
		},
	}
}
