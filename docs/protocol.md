# Piglet JSON-RPC Protocol

Piglet extensions communicate with the host via JSON-RPC 2.0 over **FD 3** (host→extension) and **FD 4** (extension→host). This leaves stdin/stdout free for the extension's own use (logging, debugging). The Go and TypeScript SDKs handle the FD wiring automatically. This document specifies the message formats and the lifecycle of an external extension.

## Message Format

All messages are newline-delimited JSON. Piglet uses a standard JSON-RPC 2.0 structure:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "methodName",
  "params": { ... },
  "result": { ... },
  "error": { "code": -32601, "message": "error message" }
}
```

- **Requests:** Must have an `id`, `method`, and optionally `params`.
- **Responses:** Must have a matching `id` and either a `result` or an `error`.
- **Notifications:** Have a `method` and optionally `params`, but no `id`. No response is expected.

## Extension Lifecycle

1. **Spawn:** The host spawns the extension process as defined in its `manifest.yaml`.
2. **Initialize:** The host sends an `initialize` request.
3. **Register:** The extension sends one or more `register/*` notifications to declare its capabilities (tools, commands, etc.).
4. **Ready:** The extension responds to the `initialize` request with its metadata.
5. **Runtime:** The host sends requests (`tool/execute`, `command/execute`, etc.) as the user interacts with the TUI. The extension can send notifications (`notify`, `log`, `showMessage`) at any time.
6. **Shutdown:** The host sends a `shutdown` notification before killing the process.

---

## Host to Extension Methods

### `initialize` (Request)
Sent immediately after the extension starts.

**Params:**
- `protocolVersion` (string): Currently "4".
- `cwd` (string): The current working directory of the host.

**Response (Result):**
- `name` (string): The extension's display name.
- `version` (string): The extension's version.

---

### `tool/execute` (Request)
Sent when the LLM calls a tool owned by the extension.

**Params:**
- `callId` (string): A unique identifier for this tool call.
- `name` (string): The name of the tool to execute.
- `args` (object): The arguments provided by the LLM.

**Response (Result):**
- `content` (array): A list of [Content Blocks](#content-blocks).
- `isError` (boolean, optional): Set to true if the tool execution failed.

---

### `command/execute` (Request)
Sent when the user runs a slash command owned by the extension.

**Params:**
- `name` (string): The name of the command.
- `args` (string): The raw arguments provided by the user (everything after the command name).

**Response (Result):**
- (empty object)

---

### `interceptor/before` (Request)
Sent before a tool (host or extension) is executed, if the extension registered an interceptor.

**Params:**
- `toolName` (string): The name of the tool about to be called.
- `args` (object): The arguments provided to the tool.

**Response (Result):**
- `allow` (boolean): Whether to allow the execution.
- `args` (object, optional): Modified arguments to pass to the tool.

---

### `interceptor/after` (Request)
Sent after a tool is executed.

**Params:**
- `toolName` (string): The name of the tool that was called.
- `details` (any): The result of the tool execution.

**Response (Result):**
- `details` (any): Optionally modified result.

---

### `event/dispatch` (Request)
Sent when a lifecycle event occurs (e.g., `EventAgentEnd`, `EventTurnEnd`).

**Params:**
- `type` (string): The event type name.
- `data` (object): Event-specific data.

**Response (Result):**
- `action` ([ActionResult](#action-results), optional): An action for the host to perform.

---

### `shortcut/handle` (Request)
Sent when a registered keyboard shortcut is pressed.

**Params:**
- `key` (string): The shortcut key (e.g., "ctrl+g").

**Response (Result):**
- `action` ([ActionResult](#action-results), optional): An action for the host to perform.

---

### `messageHook/onMessage` (Request)
Sent before a user message is sent to the LLM.

**Params:**
- `message` (string): The user's message.

**Response (Result):**
- `injection` (string, optional): Ephemeral context to inject into the system prompt for this turn only.

---

### `compact/execute` (Request)
Sent when the conversation needs to be summarized.

**Params:**
- `messages` (array): A list of messages to compact.

**Response (Result):**
- `messages` (array): The replacement list of messages (usually shorter).

---

### `$/cancelRequest` (Notification)
Sent to signal that an in-flight request should be aborted.

**Params:**
- `id` (integer): The ID of the request to cancel.

---

### `shutdown` (Notification)
Sent before the host closes the connection.

---

## Extension to Host Methods (Notifications)

Extensions should send these notifications **during the handshake phase** (after receiving `initialize` but before responding to it).

### `register/tool`
**Params:**
- `name` (string): Tool name.
- `description` (string): Tool description for the LLM.
- `parameters` (object): JSON Schema for tool arguments.
- `promptHint` (string, optional): Instructions for the LLM on when to use this tool.

### `register/command`
**Params:**
- `name` (string): Slash command name (without the `/`).
- `description` (string): Description for `/help`.

### `register/promptSection`
**Params:**
- `title` (string): Section title.
- `content` (string): Instructions to add to the system prompt.
- `order` (integer, optional): Lower numbers appear earlier.

### `register/interceptor`
**Params:**
- `name` (string): Interceptor name.
- `priority` (integer, optional): Higher numbers run first.

### `register/eventHandler`
**Params:**
- `name` (string): Handler name.
- `events` (array, optional): List of event names to observe (null = all).

### `register/shortcut`
**Params:**
- `key` (string): Key combo (e.g., "ctrl+h").
- `description` (string): Description for help.

### `register/messageHook`
**Params:**
- `name` (string): Hook name.

### `register/compactor`
**Params:**
- `name` (string): Compactor name.
- `threshold` (integer, optional): Token threshold to trigger compaction.

### `register/provider`
**Params:**
- `api` (string): The API type this extension can stream (e.g., "openai", "anthropic", "google").

---

## Extension to Host Methods (Runtime)

### `notify` (Notification)
Shows a temporary toast/notification in the TUI.
**Params:** `{ "message": "..." }`

### `showMessage` (Notification)
Adds a message to the conversation history.
**Params:** `{ "text": "..." }`

### `log` (Notification)
Writes to the host's log file.
**Params:** `{ "level": "info", "message": "..." }`

---

## Host Service Requests (Extension to Host)

These are requests that extensions can send to the host during runtime.

### Config & Auth

### `host/config.get` (Request)
Reads one or more configuration values from the host.

**Params:**
- `keys` (string[]): Configuration keys to retrieve.

**Response (Result):**
- `values` (object): Key→value map of the requested configuration.

### `host/config.readExtension` (Request)
Reads the extension configuration file at `~/.config/piglet/<name>.md`.

**Params:**
- `name` (string): Extension name.

**Response (Result):**
- `content` (string): Markdown content of the extension config file.

### `host/auth.getKey` (Request)
Retrieves an authentication key for a provider.

**Params:**
- `provider` (string): Provider name (e.g. "openai", "anthropic").

**Response (Result):**
- `key` (string): The authentication key.

---

### LLM Access

### `host/chat` (Request)
Performs a single-turn LLM call.

**Params:**
- `system` (string, optional): System prompt.
- `messages` (array): Array of `{ "role": "...", "content": "..." }` objects.
- `model` (string, optional): Model to use — "small", "default", or an explicit model ID.
- `maxTokens` (integer, optional): Maximum output tokens.

**Response (Result):**
- `text` (string): The model's response.
- `usage` (object): `{ "input": N, "output": N }` token counts.

### `host/agent.run` (Request)
Runs a full agent loop with tool use.

**Params:**
- `system` (string, optional): System prompt.
- `task` (string): The task or prompt to execute.
- `tools` (string, optional): Tool access — "background_safe" or "all".
- `model` (string, optional): Model to use.
- `maxTurns` (integer, optional): Maximum agent turns.

**Response (Result):**
- `text` (string): Final agent response.
- `turns` (integer): Number of turns taken.
- `usage` (object): `{ "input": N, "output": N }` total token counts.

---

### Tools

### `host/listTools` (Request)
Lists all tools available on the host (core tools + other extensions).

**Params:**
- `filter` (string): Either "all" or "background_safe".

**Response (Result):**
- `tools` (array): List of tool definitions.

### `host/executeTool` (Request)
Executes a tool on the host.

**Params:**
- `name` (string): Tool name.
- `args` (object): Tool arguments.

**Response (Result):**
- `content` (array): [Content Blocks](#content-blocks).
- `isError` (boolean): Whether execution failed.

---

### Session Management

### `host/conversationMessages` (Request)
Returns the current conversation's message history.

**Params:** None.

**Response (Result):**
- `messages` (array): Raw JSON array of conversation messages.

### `host/sessions` (Request)
Lists all available sessions.

**Params:** None.

**Response (Result):**
- `sessions` (array): Array of `{ "id": "...", "title": "...", "cwd": "...", "createdAt": "...", "parentId": "...", "path": "..." }` objects (`parentId` is optional).

### `host/loadSession` (Request)
Loads a session from a file path.

**Params:**
- `path` (string): Path to the session file.

**Response (Result):** Empty.

### `host/forkSession` (Request)
Forks the current session into a new child session.

**Params:** None.

**Response (Result):**
- `parentID` (string): ID of the parent session.
- `messageCount` (integer): Number of messages copied to the fork.

### `host/setSessionTitle` (Request)
Updates the current session's title.

**Params:**
- `title` (string): New session title.

**Response (Result):** Empty.

---

### Models

### `host/syncModels` (Request)
Syncs the model catalog from models.dev.

**Params:** None.

**Response (Result):**
- `updated` (integer): Number of models updated.

---

### Background Agent

### `host/runBackground` (Request)
Starts a background agent with the given prompt.

**Params:**
- `prompt` (string): The prompt to run in the background.

**Response (Result):** Empty.

### `host/cancelBackground` (Request)
Cancels the currently running background agent.

**Params:** None.

**Response (Result):** Empty.

### `host/isBackgroundRunning` (Request)
Checks whether a background agent is currently running.

**Params:** None.

**Response (Result):**
- `running` (boolean): Whether a background agent is active.

---

### Extensions

### `host/extInfos` (Request)
Returns metadata about all loaded extensions.

**Params:** None.

**Response (Result):**
- `extensions` (array): Array of `{ "name": "...", "version": "...", "kind": "...", "runtime": "...", "tools": [...], "commands": [...], "interceptors": [...], "eventHandlers": [...], "shortcuts": [...], "messageHooks": [...], "compactor": ... }` objects (all fields except `name` and `kind` are optional).

### `host/extensionsDir` (Request)
Returns the path to the extensions directory.

**Params:** None.

**Response (Result):**
- `path` (string): Absolute path to `~/.config/piglet/extensions/`.

---

### Undo

### `host/undoSnapshots` (Request)
Lists all available undo snapshots (files with pending undo state).

**Params:** None.

**Response (Result):**
- `snapshots` (object): Map of file path → snapshot size in bytes.

### `host/undoRestore` (Request)
Restores a file from its undo snapshot.

**Params:**
- `path` (string): Path of the file to restore.

**Response (Result):** Empty.

---

## Data Types

### Content Blocks
Used in tool results.

```json
{
  "type": "text",
  "text": "Hello world"
}
```
Or for images:
```json
{
  "type": "image",
  "data": "base64...",
  "mime": "image/png"
}
```

### Action Results
Returned by event handlers and shortcuts to trigger TUI actions.

```json
{
  "type": "notify",
  "payload": { "message": "Done!" }
}
```

**Supported Types:**
- `notify`: Show a TUI notification.
- `showMessage`: Display a message in the conversation.
- `setSessionTitle`: Update the current session's title.
- `setStatus`: Update a status bar key.
- `attachImage`: Attach an image to the next message.
- `detachImage`: Clear the attached image.
- `quit`: Exit Piglet.
