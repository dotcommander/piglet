package route

import (
	"fmt"
	"strings"
)

// FormatRouteResult formats a route result for human display.
func FormatRouteResult(r RouteResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Intent: %s", r.Intent.Primary)
	if r.Intent.Secondary != "" {
		fmt.Fprintf(&b, " + %s", r.Intent.Secondary)
	}
	if r.Intent.Confidence > 0 {
		fmt.Fprintf(&b, " (%.0f%%)", r.Intent.Confidence*100)
	}
	b.WriteByte('\n')

	if len(r.Domains) > 0 {
		fmt.Fprintf(&b, "Domains: %s\n", strings.Join(r.Domains, ", "))
	}

	fmt.Fprintf(&b, "Confidence: %.2f\n", r.Confidence)

	if len(r.Primary) > 0 {
		b.WriteString("\nPrimary:\n")
		for _, sc := range r.Primary {
			fmt.Fprintf(&b, "  %s (%s) — %.2f", sc.Name, sc.Type, sc.Score)
			if len(sc.Matched) > 0 {
				fmt.Fprintf(&b, " [%s]", strings.Join(sc.Matched, ", "))
			}
			b.WriteByte('\n')
		}
	}

	if len(r.Secondary) > 0 {
		b.WriteString("\nSecondary:\n")
		for _, sc := range r.Secondary {
			fmt.Fprintf(&b, "  %s (%s) — %.2f\n", sc.Name, sc.Type, sc.Score)
		}
	}

	return b.String()
}

// FormatHookContext formats routing results for injection into conversation context.
// Kept concise to minimize token overhead.
func FormatHookContext(r RouteResult) string {
	if len(r.Primary) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("[routing: ")

	if r.Intent.Primary != "" {
		b.WriteString("intent=")
		b.WriteString(r.Intent.Primary)
	}

	if len(r.Domains) > 0 {
		if r.Intent.Primary != "" {
			b.WriteString(" | ")
		}
		b.WriteString("domains=")
		b.WriteString(strings.Join(r.Domains, ","))
	}

	b.WriteString(" | relevant: ")
	names := make([]string, 0, len(r.Primary))
	for _, sc := range r.Primary {
		names = append(names, sc.Name)
	}
	b.WriteString(strings.Join(names, ", "))

	b.WriteString("]")
	return b.String()
}
