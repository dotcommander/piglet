package changelog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

// changelogState holds mutable state shared across command handlers.
type changelogState struct {
	cfg Config
	cwd string
}

func Register(e *sdk.Extension) {
	st := &changelogState{}

	e.OnInitAppend(func(x *sdk.Extension) {
		st.cwd = x.CWD()
		st.cfg = loadConfig()
		x.Log("debug", "[changelog] OnInit complete")
	})

	e.RegisterCommand(sdk.CommandDef{
		Name:        "changelog",
		Description: "Generate a changelog from git history (conventional commits)",
		Handler: func(_ context.Context, args string) error {
			args = strings.TrimSpace(args)

			write := false
			dryRun := false
			showConfig := false
			var ref string

			for _, part := range strings.Fields(args) {
				switch part {
				case "--write":
					write = true
				case "--dry-run":
					dryRun = true
				case "--config":
					showConfig = true
				default:
					if !strings.HasPrefix(part, "--") {
						ref = part
					}
				}
			}

			if showConfig {
				var b strings.Builder
				b.WriteString("Changelog type mappings:\n\n")
				for _, key := range typeOrder(st.cfg.Types) {
					tc := st.cfg.Types[key]
					fmt.Fprintf(&b, "  %s %-10s %s\n", tc.Emoji, key, tc.Label)
				}
				fmt.Fprintf(&b, "\nFallback count: %d\n", st.cfg.FallbackCount)
				e.ShowMessage(b.String())
				return nil
			}

			ref = DetectRange(st.cwd, ref, st.cfg.FallbackCount)

			commits, err := ParseCommits(st.cwd, ref)
			if err != nil {
				e.ShowMessage(fmt.Sprintf("Error: %v", err))
				return nil
			}
			if len(commits) == 0 {
				e.ShowMessage(fmt.Sprintf("No commits found in range %s", ref))
				return nil
			}

			if write || dryRun {
				repoURL := RepoURL(st.cwd)
				md := FormatMarkdown(commits, ref, repoURL, st.cfg.Types)

				if dryRun {
					e.ShowMessage("Markdown preview (dry run):\n\n" + md)
					return nil
				}

				if err := writeChangelog(st.cwd, md); err != nil {
					e.ShowMessage(fmt.Sprintf("Error writing CHANGELOG.md: %v", err))
					return nil
				}
				e.ShowMessage(fmt.Sprintf("Updated CHANGELOG.md with %d commits (%s)", len(commits), ref))
				return nil
			}

			e.ShowMessage(FormatANSI(commits, ref, st.cfg.Types))
			return nil
		},
	})
}

func writeChangelog(dir, md string) error {
	path := filepath.Join(dir, "CHANGELOG.md")

	var existing string
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}

	return xdg.WriteFileAtomic(path, []byte(md+"\n\n"+existing))
}
