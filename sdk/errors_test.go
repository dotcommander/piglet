package sdk_test

import (
	"strings"
	"testing"

	"github.com/dotcommander/piglet/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSDKToolErr(t *testing.T) {
	t.Parallel()

	result := sdk.ToolErr(sdk.ToolErrFileStale, "s", "h")

	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, sdk.ToolErrFileStale, result.ErrorCode)
	assert.Equal(t, "h", result.ErrorHint)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "text", result.Content[0].Type)
	assert.True(t, strings.HasPrefix(result.Content[0].Text, "[error:FILE_STALE] s"),
		"text should start with [error:FILE_STALE] s, got: %q", result.Content[0].Text)
}

func TestSDKToolErr_NoHint(t *testing.T) {
	t.Parallel()

	result := sdk.ToolErr(sdk.ToolErrExitNonzero, "exit code 1", "")

	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Equal(t, sdk.ToolErrExitNonzero, result.ErrorCode)
	assert.Empty(t, result.ErrorHint)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "[error:EXIT_NONZERO] exit code 1", result.Content[0].Text)
}

func TestSDKToolErr_AllStandardCodes(t *testing.T) {
	t.Parallel()

	codes := []sdk.ToolErrorCode{
		sdk.ToolErrInvalidArgs,
		sdk.ToolErrFileNotFound,
		sdk.ToolErrFileStale,
		sdk.ToolErrFileTooLarge,
		sdk.ToolErrNotRegularFile,
		sdk.ToolErrPermissionDenied,
		sdk.ToolErrNotUnique,
		sdk.ToolErrTimeout,
		sdk.ToolErrExitNonzero,
		sdk.ToolErrIO,
		sdk.ToolErrInternal,
	}

	for _, code := range codes {
		code := code
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			result := sdk.ToolErr(code, "test", "hint")
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			assert.Equal(t, code, result.ErrorCode)
			assert.True(t, strings.HasPrefix(result.Content[0].Text, "[error:"+code+"]"))
		})
	}
}
