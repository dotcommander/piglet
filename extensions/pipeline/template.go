package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TemplateContext holds all variables available for template expansion.
type TemplateContext struct {
	Params    map[string]string      // {param.<name>}
	Prev      *StepOutput            // {prev.stdout}, {prev.lines}, {prev.json.<key>}, {prev.status}
	Steps     map[string]*StepOutput // {step.<name>.stdout}, {step.<name>.status}
	Item      string                 // {item} — current each iteration value
	HasItem   bool                   // distinguishes "no item" from "item is empty string"
	LoopVars  map[string]string      // {loop.<key>} — current loop iteration values
	CWD       string                 // {cwd}
	StartTime time.Time              // consistent time reference for {date}, {timestamp}, day ranges
}

// StepOutput captures a step's output for template reference.
type StepOutput struct {
	Stdout string
	Status string // StatusOK or StatusError
}

// Clone returns a shallow copy with an independent Steps map,
// safe for concurrent use in loop iterations.
func (tc *TemplateContext) Clone() *TemplateContext {
	cp := *tc
	cp.Steps = make(map[string]*StepOutput, len(tc.Steps))
	for k, v := range tc.Steps {
		cp.Steps[k] = v
	}
	return &cp
}

// Expand replaces all {variable} placeholders in s using the context.
// Unknown variables are left as-is.
func (tc *TemplateContext) Expand(s string) string {
	if !strings.Contains(s, "{") {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	i := 0
	for i < len(s) {
		open := strings.IndexByte(s[i:], '{')
		if open < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+open])
		i += open

		close := strings.IndexByte(s[i:], '}')
		if close < 0 {
			b.WriteString(s[i:])
			break
		}
		key := s[i+1 : i+close]
		if val, ok := tc.resolve(key); ok {
			b.WriteString(val)
		} else {
			b.WriteString(s[i : i+close+1]) // leave as-is
		}
		i += close + 1
	}
	return b.String()
}

// resolve looks up a single template key.
func (tc *TemplateContext) resolve(key string) (string, bool) {
	switch {
	case strings.HasPrefix(key, "param."):
		name := key[len("param."):]
		if v, ok := tc.Params[name]; ok {
			return v, true
		}

	case key == "prev.stdout":
		if tc.Prev != nil {
			return tc.Prev.Stdout, true
		}

	// {prev.lines} — each line on its own line, trimmed for clean shell iteration
	case key == "prev.lines":
		if tc.Prev != nil {
			return strings.TrimSpace(tc.Prev.Stdout), true
		}

	case key == "prev.status":
		if tc.Prev != nil {
			return tc.Prev.Status, true
		}

	case strings.HasPrefix(key, "prev.json."):
		if tc.Prev != nil {
			field := key[len("prev.json."):]
			return jsonExtract(tc.Prev.Stdout, field), true
		}

	case strings.HasPrefix(key, "step.") && strings.HasSuffix(key, ".stdout"):
		name := key[len("step.") : len(key)-len(".stdout")]
		if so, ok := tc.Steps[name]; ok {
			return so.Stdout, true
		}

	case strings.HasPrefix(key, "step.") && strings.HasSuffix(key, ".status"):
		name := key[len("step.") : len(key)-len(".status")]
		if so, ok := tc.Steps[name]; ok {
			return so.Status, true
		}

	case key == "item":
		if tc.HasItem {
			return tc.Item, true
		}

	case strings.HasPrefix(key, "loop."):
		name := key[len("loop."):]
		if v, ok := tc.LoopVars[name]; ok {
			return v, true
		}

	case key == "cwd":
		return tc.CWD, true

	case key == "date":
		t := tc.StartTime
		if t.IsZero() {
			t = time.Now()
		}
		return t.Format("2006-01-02"), true

	case key == "timestamp":
		t := tc.StartTime
		if t.IsZero() {
			t = time.Now()
		}
		return fmt.Sprintf("%d", t.Unix()), true
	}
	return "", false
}

// jsonExtract extracts a top-level field from JSON text.
func jsonExtract(text, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return ""
		}
		return string(data)
	}
}
