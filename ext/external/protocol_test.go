package external

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageMarshalRequest(t *testing.T) {
	t.Parallel()

	id := 1
	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: ProtocolVersion,
		CWD:             "/tmp/test",
	})

	msg := Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  MethodInitialize,
		Params:  params,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "2.0", decoded.JSONRPC)
	assert.Equal(t, &id, decoded.ID)
	assert.Equal(t, MethodInitialize, decoded.Method)
}

func TestMessageMarshalNotification(t *testing.T) {
	t.Parallel()

	params, _ := json.Marshal(RegisterToolParams{
		Name:        "my_tool",
		Description: "Does things",
		Parameters:  map[string]any{"type": "object"},
	})

	msg := Message{
		JSONRPC: "2.0",
		Method:  MethodRegisterTool,
		Params:  params,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Nil(t, decoded.ID)
	assert.Equal(t, MethodRegisterTool, decoded.Method)

	var tool RegisterToolParams
	require.NoError(t, json.Unmarshal(decoded.Params, &tool))
	assert.Equal(t, "my_tool", tool.Name)
}

func TestMessageMarshalResponse(t *testing.T) {
	t.Parallel()

	id := 42
	result, _ := json.Marshal(ToolExecuteResult{
		Content: []ContentBlock{{Type: "text", Text: "hello"}},
	})

	msg := Message{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  result,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, &id, decoded.ID)
	assert.Empty(t, decoded.Method)

	var toolResult ToolExecuteResult
	require.NoError(t, json.Unmarshal(decoded.Result, &toolResult))
	assert.Len(t, toolResult.Content, 1)
	assert.Equal(t, "hello", toolResult.Content[0].Text)
}

func TestMessageMarshalError(t *testing.T) {
	t.Parallel()

	id := 5
	msg := Message{
		JSONRPC: "2.0",
		ID:      &id,
		Error:   &RPCError{Code: -32602, Message: "unknown tool"},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded Message
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.Error)
	assert.Equal(t, -32602, decoded.Error.Code)
	assert.Equal(t, "unknown tool", decoded.Error.Message)
}
