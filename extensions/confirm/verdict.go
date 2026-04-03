package confirm

import (
	"fmt"
	"strings"
)

func FormatVerdict(r *Result) string {
	if len(r.Files) == 0 {
		return "PASS — nothing to verify (no changes detected)"
	}
	if len(r.Checks) == 0 {
		return "PASS — nothing to verify (no Go packages affected)"
	}

	status := "PASS"
	if !r.Pass {
		status = "FAIL"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d checks, %d packages, %.1fs)\n",
		status, len(r.Checks), len(r.Packages), r.Elapsed)

	for _, c := range r.Checks {
		mark := "✓"
		if !c.Pass {
			mark = "✗"
		}
		fmt.Fprintf(&b, "\n  %s %-10s %.1fs", mark, c.Name, c.Elapsed)
		if !c.Pass && c.Output != "" {
			fmt.Fprintf(&b, "\n    --- output ---\n")
			for line := range strings.SplitSeq(c.Output, "\n") {
				fmt.Fprintf(&b, "    %s\n", line)
			}
			fmt.Fprintf(&b, "    --- end ---")
		}
	}

	return b.String()
}
