package sdk

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Extension
// ---------------------------------------------------------------------------

// Extension manages the JSON-RPC lifecycle for a piglet extension.
type Extension struct {
	name      string
	version   string
	cwd       string
	configDir string // extension's namespaced config dir (from host init)

	tools          map[string]*ToolDef
	commands       map[string]*CommandDef
	promptSections []PromptSectionDef
	interceptors   []InterceptorDef
	eventHandlers  []EventHandlerDef
	shortcuts      map[string]*ShortcutDef
	messageHooks   []MessageHookDef
	compactor      *CompactorDef

	inputTransformers     []InputTransformerDef
	rpcOut                *os.File           // JSON-RPC output (FD 4 or os.Stdout fallback)
	onInit                func(e *Extension) // called after initialize, before responding
	providerStreamHandler func(ctx context.Context, x *Extension, req ProviderStreamRequest) (*ProviderStreamResponse, error)
	writeMu               sync.Mutex
	cancelMu              sync.Mutex
	cancels               map[int]context.CancelFunc // request ID → cancel

	// Outgoing request tracking (extension → host)
	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[int]chan *rpcMessage // request ID → response channel

	// Event bus subscriptions (host → extension notifications)
	subsMu sync.Mutex
	subs   map[int]*Subscription // subscription ID → Subscription
}

// New creates a new extension with the given name and version.
func New(name, version string) *Extension {
	return &Extension{
		name:      name,
		version:   version,
		rpcOut:    os.Stdout,
		tools:     make(map[string]*ToolDef),
		commands:  make(map[string]*CommandDef),
		shortcuts: make(map[string]*ShortcutDef),
		cancels:   make(map[int]context.CancelFunc),
		pending:   make(map[int]chan *rpcMessage),
		subs:      make(map[int]*Subscription),
	}
}

// CWD returns the working directory provided by the host during initialization.
func (e *Extension) CWD() string { return e.cwd }

// ConfigDir returns the extension's namespaced config directory path,
// as provided by the host during initialization.
// Returns empty string if the host did not provide it.
func (e *Extension) ConfigDir() string { return e.configDir }
