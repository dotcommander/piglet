package confirm

import (
	"context"
	"os/exec"
	"time"
)

func RunTests(packages []string, root string) CheckResult {
	if len(packages) == 0 {
		return CheckResult{Name: "test", Pass: true}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	args := append([]string{"-count=1"}, packages...)
	if _, err := exec.LookPath("gotestsum"); err == nil {
		gtsArgs := append([]string{"--format", "short", "--"}, args...)
		cmd = exec.CommandContext(ctx, "gotestsum", gtsArgs...)
	} else {
		cmd = exec.CommandContext(ctx, "go", append([]string{"test"}, args...)...)
	}
	cmd.Dir = root

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()

	if err == nil {
		return CheckResult{Name: "test", Pass: true, Elapsed: elapsed}
	}
	return CheckResult{Name: "test", Pass: false, Output: capOutput(out, 8192), Elapsed: elapsed}
}
