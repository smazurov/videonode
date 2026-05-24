package cmd

import "errors"

// MCPCmd runs testenv as a stdio MCP server. Stubbed until the
// modelcontextprotocol/go-sdk wiring lands.
type MCPCmd struct{}

func (c *MCPCmd) Run(ctx *Context) error {
	return errors.New("mcp subcommand not implemented yet — host/fake CLI path is the v1 surface")
}
