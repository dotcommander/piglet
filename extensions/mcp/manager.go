package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Manager manages the lifecycle of multiple MCP server connections.
type Manager struct {
	mu       sync.Mutex
	clients  map[string]*Client
	failures map[string]string
}

// NewManager creates an empty Manager.
func NewManager() *Manager {
	return &Manager{
		clients:  make(map[string]*Client),
		failures: make(map[string]string),
	}
}

// StartAll connects to all configured MCP servers in parallel.
// Partial failures are collected; successful servers remain active.
func (m *Manager) StartAll(ctx context.Context, servers map[string]ServerConfig) []error {
	type result struct {
		name   string
		client *Client
		err    error
	}

	ch := make(chan result, len(servers))
	for name, cfg := range servers {
		go func(name string, cfg ServerConfig) {
			c, err := Connect(ctx, name, cfg)
			ch <- result{name: name, client: c, err: err}
		}(name, cfg)
	}

	var errs []error
	for range len(servers) {
		r := <-ch
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.name, r.err))
			m.mu.Lock()
			m.failures[r.name] = r.err.Error()
			m.mu.Unlock()
			continue
		}
		m.mu.Lock()
		m.clients[r.name] = r.client
		m.mu.Unlock()
	}
	return errs
}

// DiscoveredTool pairs an MCP tool with its server for tool registration.
type DiscoveredTool struct {
	ServerName  string
	ToolName    string // original MCP tool name
	FullName    string // mcp__<server>__<tool>
	Description string
	Schema      map[string]any
	Client      *Client
}

// DiscoverTools fetches and returns all tools from connected servers.
func (m *Manager) DiscoverTools(ctx context.Context) []DiscoveredTool {
	m.mu.Lock()
	defer m.mu.Unlock()

	var tools []DiscoveredTool
	for _, c := range m.clients {
		mcpTools, err := c.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range mcpTools {
			schema := map[string]any{"type": "object"}
			if len(t.InputSchema.Properties) > 0 {
				schema["properties"] = t.InputSchema.Properties
			}
			if len(t.InputSchema.Required) > 0 {
				schema["required"] = t.InputSchema.Required
			}

			tools = append(tools, DiscoveredTool{
				ServerName:  c.Name(),
				ToolName:    t.Name,
				FullName:    "mcp__" + c.Name() + "__" + t.Name,
				Description: t.Description,
				Schema:      schema,
				Client:      c,
			})
		}
	}

	sort.Slice(tools, func(i, j int) bool {
		return tools[i].FullName < tools[j].FullName
	})
	return tools
}

// Instructions collects server instructions from all connected servers.
func (m *Manager) Instructions() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var parts []string
	for _, c := range m.clients {
		if inst := c.Instructions(); inst != "" {
			parts = append(parts, fmt.Sprintf("## %s\n%s", c.Name(), inst))
		}
	}

	sort.Strings(parts)
	return strings.Join(parts, "\n\n")
}

// ServerStatus describes a connected or failed MCP server.
type ServerStatus struct {
	Name      string
	ToolCount int
	Error     string
}

// Status returns the status of all MCP servers.
func (m *Manager) Status(ctx context.Context) []ServerStatus {
	m.mu.Lock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	failures := make(map[string]string, len(m.failures))
	for k, v := range m.failures {
		failures[k] = v
	}
	m.mu.Unlock()

	out := make([]ServerStatus, 0, len(clients)+len(failures))
	for _, c := range clients {
		s := ServerStatus{Name: c.Name()}
		tools, err := c.ListTools(ctx)
		if err != nil {
			s.Error = err.Error()
		} else {
			s.ToolCount = len(tools)
		}
		out = append(out, s)
	}
	for name, errMsg := range failures {
		out = append(out, ServerStatus{Name: name, Error: errMsg})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Close terminates all MCP server connections.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, c := range m.clients {
		_ = c.Close()
		delete(m.clients, name)
	}
}
