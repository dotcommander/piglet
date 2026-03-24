// Package ext provides the extension registration surface for piglet.
//
// [App] is the central API through which all extensions — both compiled-in
// and external — register their capabilities. It supports five primitives:
//
//   - Inject: add text to the system prompt ([App.RegisterPromptSection])
//   - React: respond to triggers ([App.RegisterTool], [App.RegisterCommand])
//   - Intercept: modify or block tool calls ([App.RegisterInterceptor])
//   - Hook: process user messages before the LLM ([App.RegisterMessageHook])
//   - Observe: react to lifecycle events ([App.RegisterEventHandler])
package ext
