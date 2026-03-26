package external

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHostRoundTrip spawns a real extension process and tests the full
// initialize → register → execute → shutdown flow.
func TestHostRoundTrip(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("/opt/homebrew/bin/bun"); err != nil {
		if _, err := os.Stat("/usr/local/bin/bun"); err != nil {
			t.Skip("bun not installed, skipping integration test")
		}
	}

	// Write a minimal TS extension inline
	dir := t.TempDir()

	extScript := `
import { createReadStream, createWriteStream } from "fs";
import { createInterface } from "readline";

const rpcIn = createReadStream(null, { fd: 3 });
const rpcOut = createWriteStream(null, { fd: 4 });
const rl = createInterface({ input: rpcIn });

function send(obj) {
  rpcOut.write(JSON.stringify(obj) + "\n");
}

rl.on("line", (line) => {
  const msg = JSON.parse(line);
  if (msg.method === "initialize") {
    send({ jsonrpc: "2.0", method: "register/tool", params: { name: "test_tool", description: "A test tool", parameters: { type: "object" } } });
    send({ jsonrpc: "2.0", method: "register/command", params: { name: "test_cmd", description: "A test command" } });
    send({ jsonrpc: "2.0", id: msg.id, result: { name: "test-ext", version: "1.0.0" } });
  } else if (msg.method === "tool/execute") {
    send({ jsonrpc: "2.0", id: msg.id, result: { content: [{ type: "text", text: "executed:" + msg.params.name }] } });
  } else if (msg.method === "command/execute") {
    send({ jsonrpc: "2.0", id: msg.id, result: {} });
  } else if (msg.method === "shutdown") {
    process.exit(0);
  }
});
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ext.ts"), []byte(extScript), 0644))

	manifest := &Manifest{
		Name:    "test-ext",
		Runtime: "bun",
		Entry:   "ext.ts",
		Dir:     dir,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h := NewHost(manifest, "/tmp")
	err := h.Start(ctx)
	require.NoError(t, err)
	defer h.Stop()

	// Check registrations
	assert.Len(t, h.Tools(), 1)
	assert.Equal(t, "test_tool", h.Tools()[0].Name)

	assert.Len(t, h.Commands(), 1)
	assert.Equal(t, "test_cmd", h.Commands()[0].Name)

	// Execute tool
	result, err := h.ExecuteTool(ctx, "call-1", "test_tool", map[string]any{})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "executed:test_tool", result.Content[0].Text)

	// Execute command
	err = h.ExecuteCommand(ctx, "test_cmd", "hello")
	require.NoError(t, err)
}

func TestHostStartBadRuntime(t *testing.T) {
	t.Parallel()

	manifest := &Manifest{
		Name:    "bad-ext",
		Runtime: "/nonexistent/runtime",
		Entry:   "index.ts",
		Dir:     t.TempDir(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h := NewHost(manifest, "/tmp")
	err := h.Start(ctx)
	assert.Error(t, err)
}
