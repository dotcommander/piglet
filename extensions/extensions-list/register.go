package extlist

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

// Register registers the extensions-list extension's commands.
func Register(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "extensions",
		Description: "List loaded extensions, or install/update official extensions",
		Handler: func(ctx context.Context, args string) error {
			sub := strings.TrimSpace(args)
			if sub == "install" || sub == "update" {
				e.SendMessage("please install piglet extensions using: piglet update")
				return nil
			}

			infos, err := e.ExtInfos(ctx)
			if err != nil {
				e.ShowMessage("Failed to list extensions: " + err.Error())
				return nil
			}
			if len(infos) == 0 {
				e.ShowMessage("No extensions loaded.")
				return nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "Loaded extensions (%d)\n\n", len(infos))
			for _, info := range infos {
				label := info.Name
				if info.Version != "" {
					label = fmt.Sprintf("%s v%s", info.Name, info.Version)
				}
				fmt.Fprintf(&b, "  %s  [%s]\n", label, info.Kind)

				var caps []string
				if len(info.Tools) > 0 {
					caps = append(caps, fmt.Sprintf("tools: %s", strings.Join(info.Tools, ", ")))
				}
				if len(info.Commands) > 0 {
					caps = append(caps, fmt.Sprintf("commands: %s", strings.Join(info.Commands, ", ")))
				}
				if len(info.Interceptors) > 0 {
					caps = append(caps, fmt.Sprintf("interceptors: %s", strings.Join(info.Interceptors, ", ")))
				}
				if len(info.EventHandlers) > 0 {
					caps = append(caps, fmt.Sprintf("events: %s", strings.Join(info.EventHandlers, ", ")))
				}
				if len(info.Shortcuts) > 0 {
					caps = append(caps, fmt.Sprintf("shortcuts: %s", strings.Join(info.Shortcuts, ", ")))
				}
				if len(info.MessageHooks) > 0 {
					caps = append(caps, fmt.Sprintf("hooks: %s", strings.Join(info.MessageHooks, ", ")))
				}
				if info.Compactor != "" {
					caps = append(caps, fmt.Sprintf("compactor: %s", info.Compactor))
				}
				for _, c := range caps {
					fmt.Fprintf(&b, "    %s\n", c)
				}
			}
			b.WriteString("\nUse /extensions install to install official extensions.")
			e.ShowMessage(b.String())
			return nil
		},
	})
}
