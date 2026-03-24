// Package core implements the agent loop, streaming, and message types.
//
// Core is deliberately minimal and imports nothing from piglet — all
// capabilities (tools, commands, prompt sections) are provided by
// extensions through the [ext] package. The agent loop sends messages
// to a [StreamProvider], executes tool calls, and repeats until the
// model is done.
package core
