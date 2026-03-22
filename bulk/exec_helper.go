package bulk

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// expandTemplate replaces {path}, {name}, {dir}, {basename} in the template.
func expandTemplate(tmpl string, item Item) string {
	dir := filepath.Dir(item.Path)
	base := filepath.Base(item.Path)
	ext := filepath.Ext(base)
	nameNoExt := strings.TrimSuffix(base, ext)

	r := strings.NewReplacer(
		"{path}", item.Path,
		"{name}", item.Name,
		"{dir}", dir,
		"{basename}", nameNoExt,
	)
	return r.Replace(tmpl)
}

// shellExec runs a shell command in the given directory and returns stdout.
// On error, returns stderr content as the error message.
func shellExec(ctx context.Context, dir, shell, command string) (string, error) {
	if shell == "" {
		shell = "sh"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s", errMsg)
	}

	return stdout.String(), nil
}
