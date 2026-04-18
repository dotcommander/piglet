package external

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// providerResolverFn resolves a model specifier to a StreamProvider.
type providerResolverFn func(model string) (core.StreamProvider, error)

// Host manages a single external extension process.
type Host struct {
	manifest          *Manifest
	cwd               string
	app               *ext.App // nil until bridge wires it
	resolveProviderFn providerResolverFn
	undoSnapshotsFn   func() (map[string][]byte, error)

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	rpcRead io.Closer // ext→host read end (for cleanup)
	stdout  *bufio.Scanner

	ctx    context.Context
	cancel context.CancelFunc

	writeMu   sync.Mutex // protects stdin writes
	pendingMu sync.Mutex // protects pending map
	nextID    atomic.Int64
	pending   map[int]chan *Message // pending request ID → response channel
	closed    chan struct{}
	closeOnce sync.Once
	readDone  chan struct{} // closed when readLoop exits

	// Registrations collected during handshake
	tools             []RegisterToolParams
	commands          []RegisterCommandParams
	promptSections    []RegisterPromptSectionParams
	interceptors      []RegisterInterceptorParams
	eventHandlers     []RegisterEventHandlerParams
	shortcuts         []RegisterShortcutParams
	messageHooks      []RegisterMessageHookParams
	compactor         *RegisterCompactorParams
	inputTransformers []RegisterInputTransformerParams
	providers         []RegisterProviderParams // collected during handshake
	deltaMu           sync.Mutex
	deltaChans        map[int]chan ProviderDeltaParams // requestId → delta channel
	subsMu            sync.Mutex
	subscriptions     map[int]func() // subscription ID → unsubscribe function
}

// NewHost creates a host for the given manifest.
func NewHost(m *Manifest, cwd string) *Host {
	return &Host{
		manifest:      m,
		cwd:           cwd,
		pending:       make(map[int]chan *Message),
		closed:        make(chan struct{}),
		readDone:      make(chan struct{}),
		deltaChans:    make(map[int]chan ProviderDeltaParams),
		subscriptions: make(map[int]func()),
	}
}

// SetApp wires the host to the main application for runtime notifications.
func (h *Host) SetApp(app *ext.App) { h.app = app }

// SetProviderResolver sets the function used to resolve a model to a StreamProvider.
func (h *Host) SetProviderResolver(fn providerResolverFn) { h.resolveProviderFn = fn }

// SetUndoSnapshots sets the function used to retrieve undo snapshots.
func (h *Host) SetUndoSnapshots(fn func() (map[string][]byte, error)) { h.undoSnapshotsFn = fn }

// Name returns the extension name from the manifest.
func (h *Host) Name() string { return h.manifest.Name }

// Closed returns a channel that is closed when the host process exits.
func (h *Host) Closed() <-chan struct{} { return h.closed }

// releaseDeltaChan removes a delta channel from the map, stopping further sends.
func (h *Host) releaseDeltaChan(id int) {
	h.deltaMu.Lock()
	delete(h.deltaChans, id)
	h.deltaMu.Unlock()
}

// Tools returns the tools registered during handshake.
func (h *Host) Tools() []RegisterToolParams { return h.tools }

// Commands returns the commands registered during handshake.
func (h *Host) Commands() []RegisterCommandParams { return h.commands }

// PromptSections returns the prompt sections registered during handshake.
func (h *Host) PromptSections() []RegisterPromptSectionParams { return h.promptSections }

// Interceptors returns the interceptors registered during handshake.
func (h *Host) Interceptors() []RegisterInterceptorParams { return h.interceptors }

// EventHandlers returns the event handlers registered during handshake.
func (h *Host) EventHandlers() []RegisterEventHandlerParams { return h.eventHandlers }

// Shortcuts returns the shortcuts registered during handshake.
func (h *Host) Shortcuts() []RegisterShortcutParams { return h.shortcuts }

// MessageHooks returns the message hooks registered during handshake.
func (h *Host) MessageHooks() []RegisterMessageHookParams { return h.messageHooks }

// Compactor returns the compactor registered during handshake, or nil.
func (h *Host) Compactor() *RegisterCompactorParams { return h.compactor }

// InputTransformers returns the input transformers registered during handshake.
func (h *Host) InputTransformers() []RegisterInputTransformerParams { return h.inputTransformers }

// Providers returns the providers registered during handshake.
func (h *Host) Providers() []RegisterProviderParams { return h.providers }
