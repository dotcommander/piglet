package scaffold

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

// Register registers the scaffold extension's commands.
func Register(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "ext-init",
		Description: "Scaffold a new extension",
		Handler: func(ctx context.Context, args string) error {
			name := strings.TrimSpace(args)
			if name == "" {
				e.ShowMessage("Usage: /ext-init <name>\nExample: /ext-init my-tool")
				return nil
			}

			extDir, err := e.ExtensionsDir(ctx)
			if err != nil {
				return fmt.Errorf("extensions dir: %w", err)
			}

			dir := filepath.Join(extDir, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}

			manifest := fmt.Sprintf(`name: %s
version: 0.1.0
runtime: bun
entry: index.ts
capabilities:
  - tools
  - commands
`, name)

			r := strings.NewReplacer("{{NAME}}", name)
			indexTS := r.Replace(`import { piglet } from "@piglet/sdk";

piglet.setInfo("{{NAME}}", "0.1.0");

piglet.registerTool({
  name: "{{NAME}}_hello",
  description: "A greeting tool",
  parameters: {
    type: "object",
    properties: {
      name: { type: "string", description: "Name to greet" },
    },
    required: ["name"],
  },
  execute: async (args) => {
    return { text: "Hello, " + args.name + "!" };
  },
});

piglet.registerCommand({
  name: "{{NAME}}",
  description: "Run {{NAME}}",
  handler: async (args) => {
    piglet.notify("{{NAME}}: " + (args || "no args"));
  },
});
`)

			if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
				return fmt.Errorf("write manifest: %w", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "index.ts"), []byte(indexTS), 0644); err != nil {
				return fmt.Errorf("write index.ts: %w", err)
			}

			e.ShowMessage(fmt.Sprintf("Created extension at %s/\n\nFiles:\n  manifest.yaml — extension config\n  index.ts      — your code\n\nInstall SDK: cd %s && bun add @piglet/sdk\nRestart piglet to load.", dir, dir))
			return nil
		},
	})
}
