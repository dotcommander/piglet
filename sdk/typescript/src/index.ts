/**
 * Piglet Extension SDK for TypeScript/JavaScript
 *
 * Communicates with the piglet host via JSON-RPC over stdin/stdout.
 *
 * Usage:
 *   import { piglet } from "@piglet/sdk";
 *
 *   piglet.registerTool({
 *     name: "my_tool",
 *     description: "Does something",
 *     parameters: { type: "object", properties: {} },
 *     execute: async (args) => ({ text: "result" }),
 *   });
 *
 *   piglet.registerCommand({
 *     name: "greet",
 *     description: "Say hello",
 *     handler: async (args) => { piglet.notify("Hello!"); },
 *   });
 */

import { createInterface } from "readline";

// ---------------------------------------------------------------------------
// Protocol constants (must match ext/external/protocol.go)
// ---------------------------------------------------------------------------

const Method = {
  Initialize: "initialize",
  Shutdown: "shutdown",
  RegisterTool: "register/tool",
  RegisterCommand: "register/command",
  RegisterPromptSection: "register/promptSection",
  ToolExecute: "tool/execute",
  CommandExecute: "command/execute",
  Notify: "notify",
  Log: "log",
  ShowMessage: "showMessage",
} as const;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface RPCMessage {
  jsonrpc: "2.0";
  id?: number;
  method?: string;
  params?: unknown;
  result?: unknown;
  error?: { code: number; message: string };
}

interface ToolDef {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  promptHint?: string;
  execute: (args: Record<string, unknown>) => Promise<ToolResult>;
}

type ToolResult = { text: string } | { content: ContentBlock[] };

interface ContentBlock {
  type: "text" | "image";
  text?: string;
  data?: string;
  mime?: string;
}

interface CommandDef {
  name: string;
  description: string;
  immediate?: boolean;
  handler: (args: string) => Promise<void>;
}

interface PromptSectionDef {
  title: string;
  content: string;
  order?: number;
}

interface InitParams {
  protocolVersion: string;
  cwd: string;
}

interface ToolExecuteParams {
  callId: string;
  name: string;
  args: Record<string, unknown>;
}

interface CommandExecuteParams {
  name: string;
  args: string;
}

// ---------------------------------------------------------------------------
// Piglet SDK
// ---------------------------------------------------------------------------

class PigletSDK {
  private tools = new Map<string, ToolDef>();
  private commands = new Map<string, CommandDef>();
  private name = "unnamed";
  private version = "0.0.0";
  private cwd = ".";
  private ready = false;

  constructor() {
    this.startReadLoop();
  }

  /** Set extension metadata (call before registrations). */
  setInfo(name: string, version: string): void {
    this.name = name;
    this.version = version;
  }

  /** Register a tool the LLM can call. */
  registerTool(tool: ToolDef): void {
    this.tools.set(tool.name, tool);
    if (this.ready) {
      this.sendNotification(Method.RegisterTool, {
        name: tool.name,
        description: tool.description,
        parameters: tool.parameters,
        promptHint: tool.promptHint,
      });
    }
  }

  /** Register a slash command. */
  registerCommand(cmd: CommandDef): void {
    this.commands.set(cmd.name, cmd);
    if (this.ready) {
      const params: Record<string, any> = {
        name: cmd.name,
        description: cmd.description,
      };
      if (cmd.immediate) {
        params.immediate = true;
      }
      this.sendNotification(Method.RegisterCommand, params);
    }
  }

  /** Register a prompt section. */
  registerPromptSection(section: PromptSectionDef): void {
    this.sendNotification(Method.RegisterPromptSection, section);
  }

  /** Send a notification to the TUI. */
  notify(message: string): void {
    this.sendNotification(Method.Notify, { message });
  }

  /** Display a message in the conversation. */
  showMessage(text: string): void {
    this.sendNotification(Method.ShowMessage, { text });
  }

  /** Log a message to the host. */
  log(level: "debug" | "info" | "warn" | "error", message: string): void {
    this.sendNotification(Method.Log, { level, message });
  }

  /** Get the working directory (available after initialize). */
  getCwd(): string {
    return this.cwd;
  }

  // -------------------------------------------------------------------------
  // Internal
  // -------------------------------------------------------------------------

  private startReadLoop(): void {
    const rl = createInterface({ input: process.stdin });
    rl.on("line", (line: string) => {
      if (!line.trim()) return;
      try {
        const msg: RPCMessage = JSON.parse(line);
        this.handleMessage(msg);
      } catch {
        // Ignore malformed messages
      }
    });
    rl.on("close", () => process.exit(0));
  }

  private handleMessage(msg: RPCMessage): void {
    if (msg.method && msg.id !== undefined) {
      // Request from host
      this.handleRequest(msg);
    }
  }

  private async handleRequest(msg: RPCMessage): Promise<void> {
    try {
      switch (msg.method) {
        case Method.Initialize:
          await this.handleInitialize(msg);
          break;
        case Method.ToolExecute:
          await this.handleToolExecute(msg);
          break;
        case Method.CommandExecute:
          await this.handleCommandExecute(msg);
          break;
        case Method.Shutdown:
          this.sendResponse(msg.id!, null);
          process.exit(0);
          break;
        default:
          this.sendError(msg.id!, -32601, `Unknown method: ${msg.method}`);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.sendError(msg.id!, -32603, message);
    }
  }

  private async handleInitialize(msg: RPCMessage): Promise<void> {
    const params = msg.params as InitParams;
    this.cwd = params.cwd;
    this.ready = true;

    // Send registrations
    for (const tool of this.tools.values()) {
      this.sendNotification(Method.RegisterTool, {
        name: tool.name,
        description: tool.description,
        parameters: tool.parameters,
        promptHint: tool.promptHint,
      });
    }
    for (const cmd of this.commands.values()) {
      const params: Record<string, any> = {
        name: cmd.name,
        description: cmd.description,
      };
      if (cmd.immediate) {
        params.immediate = true;
      }
      this.sendNotification(Method.RegisterCommand, params);
    }

    // Respond to initialize
    this.sendResponse(msg.id!, { name: this.name, version: this.version });
  }

  private async handleToolExecute(msg: RPCMessage): Promise<void> {
    const params = msg.params as ToolExecuteParams;
    const tool = this.tools.get(params.name);
    if (!tool) {
      this.sendError(msg.id!, -32602, `Unknown tool: ${params.name}`);
      return;
    }

    const result = await tool.execute(params.args);

    let content: ContentBlock[];
    if ("text" in result) {
      content = [{ type: "text", text: result.text }];
    } else {
      content = result.content;
    }

    this.sendResponse(msg.id!, { content, isError: false });
  }

  private async handleCommandExecute(msg: RPCMessage): Promise<void> {
    const params = msg.params as CommandExecuteParams;
    const cmd = this.commands.get(params.name);
    if (!cmd) {
      this.sendError(msg.id!, -32602, `Unknown command: ${params.name}`);
      return;
    }

    await cmd.handler(params.args);
    this.sendResponse(msg.id!, {});
  }

  private sendNotification(method: string, params: unknown): void {
    this.write({ jsonrpc: "2.0", method, params });
  }

  private sendResponse(id: number, result: unknown): void {
    this.write({ jsonrpc: "2.0", id, result });
  }

  private sendError(id: number, code: number, message: string): void {
    this.write({ jsonrpc: "2.0", id, error: { code, message } });
  }

  private write(msg: RPCMessage): void {
    process.stdout.write(JSON.stringify(msg) + "\n");
  }
}

/** The singleton piglet SDK instance. */
export const piglet = new PigletSDK();
