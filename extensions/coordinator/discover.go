package coordinator

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

// extName is the extension name used for self-identification and display.
const extName = "coordinator"

// Capability represents a discovered extension capability.
type Capability struct {
	Extension string
	Tools     []string
	Commands  []string
}

// DiscoverCapabilities queries the host for all loaded extensions and their tools.
func DiscoverCapabilities(ctx context.Context, ext *sdk.Extension) ([]Capability, error) {
	infos, err := ext.ExtInfos(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}

	return filterCapabilities(infos), nil
}

// filterCapabilities extracts capabilities from extension info, skipping self and toolless extensions.
func filterCapabilities(infos []sdk.ExtInfo) []Capability {
	caps := make([]Capability, 0, len(infos))
	for _, info := range infos {
		if info.Name == extName {
			continue // skip self
		}
		if len(info.Tools) == 0 && len(info.Commands) == 0 {
			continue
		}
		caps = append(caps, Capability{
			Extension: info.Name,
			Tools:     info.Tools,
			Commands:  info.Commands,
		})
	}
	return caps
}

// FormatCapabilities returns a text summary of discovered capabilities for LLM consumption.
func FormatCapabilities(caps []Capability) string {
	if len(caps) == 0 {
		return "No extension capabilities discovered."
	}

	var b strings.Builder
	for _, c := range caps {
		fmt.Fprintf(&b, "%s:", c.Extension)
		if len(c.Tools) > 0 {
			fmt.Fprintf(&b, " tools=[%s]", strings.Join(c.Tools, ", "))
		}
		if len(c.Commands) > 0 {
			fmt.Fprintf(&b, " commands=[%s]", strings.Join(c.Commands, ", "))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
