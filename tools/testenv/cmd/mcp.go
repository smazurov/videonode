package cmd

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/smazurov/videonode/tools/testenv/internal/mcpsrv"
)

// MCPCmd runs testenv as a stdio MCP server.
type MCPCmd struct{}

func (c *MCPCmd) Run(ctx *Context) error {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "testenv", Version: Version},
		nil,
	)
	mcpsrv.Register(server, ctx.StatePath)
	return server.Run(ctx.Ctx, &mcp.StdioTransport{})
}
