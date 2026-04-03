package confirm

import (
	"context"
	"os/exec"
	"time"
)

func Lint(files []string, root string) CheckResult {
	if len(files) == 0 {
		return CheckResult{Name: "lint", Pass: true}
	}

	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return CheckResult{Name: "lint", Pass: true, Output: "skipped: golangci-lint not found"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "golangci-lint", "run", "--new-from-rev=HEAD~1", "--timeout=60s")
	cmd.Dir = root

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()

	if err == nil {
		return CheckResult{Name: "lint", Pass: true, Elapsed: elapsed}
	}
	return CheckResult{Name: "lint", Pass: false, Output: capOutput(out, 4096), Elapsed: elapsed}
}
