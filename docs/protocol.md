# Piglet JSON-RPC Protocol

Piglet extensions communicate with the host via JSON-RPC 2.0 over `stdin` and `stdout`. This document specifies the message formats and the lifecycle of an external extension.

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
- `protocolVersion` (string): Currently "2".
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
