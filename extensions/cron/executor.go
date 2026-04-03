package cron

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"
)

// ExecuteResult holds the outcome of a task execution.
type ExecuteResult struct {
	Success    bool
	DurationMs int64
	Error      string
	Output     string // truncated stdout/stderr for shell, response for webhook
}

// Execute runs a task's action with the given context and timeout.
func Execute(ctx context.Context, name string, task TaskConfig) ExecuteResult {
	timeout := 5 * time.Minute
	if task.Timeout != "" {
		if d, err := time.ParseDuration(task.Timeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	var result ExecuteResult
	switch task.Action {
	case "shell":
		result = executeShell(ctx, task)
	case "prompt":
		result = executePrompt(ctx, task)
	case "webhook":
		result = executeWebhook(ctx, task)
	default:
		result = ExecuteResult{Error: fmt.Sprintf("unknown action %q", task.Action)}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

func executeShell(ctx context.Context, task TaskConfig) ExecuteResult {
	if task.Command == "" {
		return ExecuteResult{Error: "shell action requires command"}
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", task.Command)
	if task.WorkDir != "" {
		cmd.Dir = task.WorkDir
	}
	if len(task.Env) > 0 {
		cmd.Env = append(cmd.Environ(), mapToEnv(task.Env)...)
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()
	// Truncate output to 4KB for history storage.
	const maxOutput = 4096
	if len(output) > maxOutput {
		output = output[:maxOutput] + "...(truncated)"
	}

	if err != nil {
		return ExecuteResult{Error: err.Error(), Output: output}
	}
	return ExecuteResult{Success: true, Output: output}
}

func executePrompt(ctx context.Context, task TaskConfig) ExecuteResult {
	if task.Prompt == "" {
		return ExecuteResult{Error: "prompt action requires prompt"}
	}

	// Shell out to piglet CLI with --prompt flag.
	cmd := exec.CommandContext(ctx, "piglet", "--prompt", task.Prompt)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()
	const maxOutput = 4096
	if len(output) > maxOutput {
		output = output[:maxOutput] + "...(truncated)"
	}

	if err != nil {
		return ExecuteResult{Error: err.Error(), Output: output}
	}
	return ExecuteResult{Success: true, Output: output}
}

func executeWebhook(ctx context.Context, task TaskConfig) ExecuteResult {
	if task.URL == "" {
		return ExecuteResult{Error: "webhook action requires url"}
	}

	method := task.Method
	if method == "" {
		method = http.MethodPost
	}

	var body *bytes.Reader
	if task.Body != "" {
		body = bytes.NewReader([]byte(task.Body))
	} else {
		body = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, task.URL, body)
	if err != nil {
		return ExecuteResult{Error: fmt.Sprintf("create request: %s", err)}
	}

	for k, v := range task.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ExecuteResult{Error: fmt.Sprintf("request failed: %s", err)}
	}
	// Drain body so the TCP connection can be returned to the pool.
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ExecuteResult{Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	return ExecuteResult{Success: true, Output: fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

func mapToEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}
