package bulk

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/safeexec"
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
// The subprocess environment is filtered via safeexec.FilterEnv.
func shellExec(ctx context.Context, dir, shell, command string) (string, error) {
	if shell == "" {
		shell = "sh"
	}
	return safeexec.Run(ctx, dir, 0, shell, "-c", command)
}
