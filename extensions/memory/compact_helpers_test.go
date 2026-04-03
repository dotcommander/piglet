package memory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseXMLTag_Empty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, parseXMLTag("", "read-files"))
	assert.Nil(t, parseXMLTag("no tags here", "read-files"))
}

func TestParseXMLTag_SingleFile(t *testing.T) {
	t.Parallel()
	text := "summary\n<read-files>\nmain.go\n</read-files>"
	got := parseXMLTag(text, "read-files")
	assert.Equal(t, []string{"main.go"}, got)
}

func TestParseXMLTag_MultipleFiles(t *testing.T) {
	t.Parallel()
	text := "<modified-files>\na.go\nb.go\nc.go\n</modified-files>"
	got := parseXMLTag(text, "modified-files")
	assert.Equal(t, []string{"a.go", "b.go", "c.go"}, got)
}

func TestParseXMLTag_SkipsBlankLines(t *testing.T) {
	t.Parallel()
	text := "<read-files>\n\na.go\n\nb.go\n\n</read-files>"
	got := parseXMLTag(text, "read-files")
	assert.Equal(t, []string{"a.go", "b.go"}, got)
}

func TestParseXMLTag_UnclosedTag(t *testing.T) {
	t.Parallel()
	text := "<read-files>\na.go\n"
	assert.Nil(t, parseXMLTag(text, "read-files"))
}

func TestStripXMLTag_NoTag(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", stripXMLTag("hello", "read-files"))
}

func TestStripXMLTag_RemovesTag(t *testing.T) {
	t.Parallel()
	text := "before\n<read-files>\na.go\n</read-files>\nafter"
	got := stripXMLTag(text, "read-files")
	assert.Equal(t, "before\n\nafter", got)
}

func TestDedup_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, dedup(nil))
}

func TestDedup_NoDuplicates(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"a", "b", "c"}, dedup([]string{"a", "b", "c"}))
}

func TestDedup_WithDuplicates(t *testing.T) {
	t.Parallel()
	got := dedup([]string{"a", "b", "a", "c", "b"})
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestMergeFileLists_NoPrior(t *testing.T) {
	t.Parallel()
	summary := "some summary"
	got := mergeFileLists(summary, nil, nil)
	assert.Equal(t, summary, got)
}

func TestMergeFileLists_AddsNewTags(t *testing.T) {
	t.Parallel()
	got := mergeFileLists("summary", []string{"a.go"}, []string{"b.go"})
	assert.Contains(t, got, "<read-files>")
	assert.Contains(t, got, "a.go")
	assert.Contains(t, got, "<modified-files>")
	assert.Contains(t, got, "b.go")
}

func TestMergeFileLists_MergesExisting(t *testing.T) {
	t.Parallel()
	summary := "summary\n<read-files>\na.go\n</read-files>\n<modified-files>\nx.go\n</modified-files>"
	got := mergeFileLists(summary, []string{"b.go"}, []string{"y.go"})

	readFiles := parseXMLTag(got, "read-files")
	modFiles := parseXMLTag(got, "modified-files")

	assert.Contains(t, readFiles, "a.go")
	assert.Contains(t, readFiles, "b.go")
	assert.Contains(t, modFiles, "x.go")
	assert.Contains(t, modFiles, "y.go")
}

func TestMergeFileLists_Deduplicates(t *testing.T) {
	t.Parallel()
	summary := "summary\n<read-files>\na.go\n</read-files>"
	got := mergeFileLists(summary, []string{"a.go", "b.go"}, nil)

	readFiles := parseXMLTag(got, "read-files")
	assert.Equal(t, []string{"a.go", "b.go"}, readFiles)
}

func TestExtractPriorFileLists_Empty(t *testing.T) {
	t.Parallel()
	read, mod := extractPriorFileLists(nil)
	assert.Empty(t, read)
	assert.Empty(t, mod)
}

func TestExtractPriorFileLists_FindsTags(t *testing.T) {
	t.Parallel()
	content := "Summary\n<read-files>\nfoo.go\n</read-files>\n<modified-files>\nbar.go\n</modified-files>"
	data, err := json.Marshal(map[string]string{"content": content})
	require.NoError(t, err)

	msgs := []wireMsg{{Type: "user", Data: data}}
	read, mod := extractPriorFileLists(msgs)

	assert.Contains(t, read, "foo.go")
	assert.Contains(t, mod, "bar.go")
}

func TestExtractPriorFileLists_DedupsAcrossMessages(t *testing.T) {
	t.Parallel()
	mk := func(content string) wireMsg {
		data, _ := json.Marshal(map[string]string{"content": content})
		return wireMsg{Type: "user", Data: data}
	}

	msgs := []wireMsg{
		mk("<read-files>\na.go\nb.go\n</read-files>"),
		mk("<read-files>\nb.go\nc.go\n</read-files>"),
	}

	read, _ := extractPriorFileLists(msgs)
	// Should have a, b, c — no duplicates
	assert.Len(t, read, 3)
}

func makeToolResultMsg(toolName, text string) wireMsg {
	tr := wireToolResult{
		ToolName: toolName,
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: text}},
	}
	data, _ := json.Marshal(tr)
	return wireMsg{Type: "tool_result", Data: data}
}

func TestTruncateToolResults_NoOp(t *testing.T) {
	t.Parallel()
	msgs := []wireMsg{makeToolResultMsg("read", "short")}
	truncateToolResults(msgs, 2000)

	var tr wireToolResult
	require.NoError(t, json.Unmarshal(msgs[0].Data, &tr))
	assert.Equal(t, "short", tr.Content[0].Text)
}

func TestTruncateToolResults_Truncates(t *testing.T) {
	t.Parallel()
	longText := make([]rune, 3000)
	for i := range longText {
		longText[i] = 'x'
	}
	msgs := []wireMsg{makeToolResultMsg("bash", string(longText))}
	truncateToolResults(msgs, 2000)

	var tr wireToolResult
	require.NoError(t, json.Unmarshal(msgs[0].Data, &tr))
	// 2000 runes + truncation suffix
	assert.Less(t, len([]rune(tr.Content[0].Text)), 2100)
	assert.Contains(t, tr.Content[0].Text, "[...truncated for compaction]")
}

func TestTruncateToolResults_SkipsNonToolResult(t *testing.T) {
	t.Parallel()
	data, _ := json.Marshal(map[string]string{"content": "hello"})
	msgs := []wireMsg{{Type: "user", Data: data}}
	// Should not panic or modify
	truncateToolResults(msgs, 2000)
	assert.Equal(t, "user", msgs[0].Type)
}
