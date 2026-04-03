package mcp

import (
	"context"
	"fmt"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

const connectTimeout = 30 * time.Second

// Client wraps a single MCP server connection.
type Client struct {
	name    string
	cli     mcpclient.MCPClient
	initRes *mcp.InitializeResult
}

// Connect establishes an MCP connection to a server.
func Connect(ctx context.Context, name string, cfg ServerConfig) (*Client, error) {
	c := &Client{name: name}

	var cli *mcpclient.Client
	var err error

	switch cfg.Type {
	case "http":
		headers := ExpandHeaders(cfg.Headers)
		cli, err = mcpclient.NewSSEMCPClient(cfg.URL, mcpclient.WithHeaders(headers))
	default: // stdio
		var env []string
		if len(cfg.Env) > 0 {
			env = ExpandEnv(cfg.Env)
		}
		cli, err = mcpclient.NewStdioMCPClient(cfg.Command, env, cfg.Args...)
	}
	if err != nil {
		return nil, fmt.Errorf("create client %s: %w", name, err)
	}

	c.cli = cli

	// Initialize with timeout
	initCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	initRes, err := cli.Initialize(initCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: "2025-03-26",
			ClientInfo: mcp.Implementation{
				Name:    "piglet",
				Version: "0.1.0",
			},
		},
	})
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("initialize %s: %w", name, err)
	}

	c.initRes = initRes
	return c, nil
}

// Name returns the server name.
func (c *Client) Name() string { return c.name }

// ListTools fetches the tool list from the server.
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	result, err := c.cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list tools from %s: %w", c.name, err)
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	return c.cli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	})
}

// Instructions returns server instructions from the initialize result.
func (c *Client) Instructions() string {
	if c.initRes != nil {
		return c.initRes.Instructions
	}
	return ""
}

// Close terminates the MCP session.
func (c *Client) Close() error {
	return c.cli.Close()
}
