// Package sdk provides the Go Extension SDK for building piglet extensions.
//
// Extensions are standalone binaries that communicate with the piglet host
// via JSON-RPC v2 over stdin/stdout. They can register tools, commands,
// prompt sections, interceptors, event handlers, shortcuts, and message hooks.
//
// Basic usage:
//
//	func main() {
//	    e := sdk.New("my-extension", "0.1.0")
//	    e.RegisterTool(sdk.ToolDef{
//	        Name:        "my_tool",
//	        Description: "Does something useful",
//	        Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
//	            return sdk.TextResult("done"), nil
//	        },
//	    })
//	    e.Run()
//	}
package sdk
