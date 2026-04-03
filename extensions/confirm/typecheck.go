package confirm

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type CheckResult struct {
	Name    string  `json:"name"`
	Pass    bool    `json:"pass"`
	Output  string  `json:"output,omitempty"`
	Elapsed float64 `json:"elapsed_s"`
}

func capOutput(b []byte, max int) string {
	if len(b) > max {
		b = b[:max]
	}
	return strings.TrimSpace(string(b))
}

func TypeCheck(packages []string, root string) CheckResult {
	if len(packages) == 0 {
		return CheckResult{Name: "typecheck", Pass: true}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	args := append([]string{"build"}, packages...)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = root

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()

	if err == nil {
		return CheckResult{Name: "typecheck", Pass: true, Elapsed: elapsed}
	}
	return CheckResult{Name: "typecheck", Pass: false, Output: capOutput(out, 4096), Elapsed: elapsed}
}
