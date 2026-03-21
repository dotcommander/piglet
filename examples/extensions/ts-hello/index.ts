import { piglet } from "../../../sdk/typescript/src/index.ts";

piglet.setInfo("hello", "0.1.0");

// Register a tool the LLM can call
piglet.registerTool({
  name: "hello_world",
  description: "Returns a friendly greeting",
  parameters: {
    type: "object",
    properties: {
      name: {
        type: "string",
        description: "Name to greet",
      },
    },
    required: ["name"],
  },
  promptHint: "Use hello_world to greet someone by name",
  execute: async (args) => {
    const name = args.name as string;
    return { text: `Hello, ${name}! Welcome to piglet.` };
  },
});

// Register a slash command
piglet.registerCommand({
  name: "wave",
  description: "Wave at someone",
  handler: async (args) => {
    const target = args.trim() || "world";
    piglet.notify(`👋 Waving at ${target}!`);
  },
});
