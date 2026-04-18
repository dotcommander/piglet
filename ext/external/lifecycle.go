package external

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/dotcommander/piglet/config"
)

// Start spawns the extension process, performs the initialize handshake,
// and collects registrations. Returns after the extension sends initialize result.
func (h *Host) Start(ctx context.Context) error {
	h.ctx, h.cancel = context.WithCancel(ctx)

	bin, args := h.manifest.RuntimeCommand()
	h.cmd = exec.CommandContext(ctx, bin, args...)
	h.cmd.Dir = h.manifest.Dir

	if err := h.connectPipes(); err != nil {
		return err
	}

	if err := h.cmd.Start(); err != nil {
		h.stdin.Close()
		h.rpcRead.Close()
		return fmt.Errorf("start %s: %w", h.manifest.Name, err)
	}

	// Start reading messages in background
	go h.readLoop()

	// Send initialize with a 10-second timeout to prevent hanging
	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	t0 := time.Now()
	extCfgDir, _ := config.ExtensionConfigDir(h.manifest.Name)
	result, err := h.request(initCtx, MethodInitialize, InitializeParams{
		ProtocolVersion: ProtocolVersion,
		CWD:             h.cwd,
		ConfigDir:       extCfgDir,
	})
	if err != nil {
		slog.Warn("extension init failed", "name", h.manifest.Name, "elapsed", time.Since(t0).Round(time.Millisecond))
		h.Stop()
		return fmt.Errorf("initialize %s: %w", h.manifest.Name, err)
	}

	var initResult InitializeResult
	if result.Result != nil {
		if err := json.Unmarshal(result.Result, &initResult); err != nil {
			slog.Warn("unmarshal init result", "name", h.manifest.Name, "err", err)
		}
	}

	slog.Debug("extension initialized", "name", h.manifest.Name, "ext_version", initResult.Version, "elapsed", time.Since(t0).Round(time.Millisecond))
	return nil
}

// connectPipes creates two anonymous pipe pairs for JSON-RPC communication
// (FD 3/4 in the child process), wires them into h.cmd.ExtraFiles, and
// assigns h.stdin, h.rpcRead, and h.stdout on the host side.
// The child-side ends are closed after cmd.Start() returns.
func (h *Host) connectPipes() error {
	// Pair 1: host→ext (host writes, child reads on FD 3)
	extRead, hostWrite, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create host→ext pipe: %w", err)
	}
	// Pair 2: ext→host (child writes on FD 4, host reads)
	hostRead, extWrite, err := os.Pipe()
	if err != nil {
		extRead.Close()
		hostWrite.Close()
		return fmt.Errorf("create ext→host pipe: %w", err)
	}

	h.cmd.ExtraFiles = []*os.File{extRead, extWrite} // become FD 3, FD 4
	h.cmd.Env = append(os.Environ(), "PIGLET_FD=1")
	h.cmd.Stdin = nil                                            // extensions don't read stdin
	h.cmd.Stdout = &logWriter{name: h.manifest.Name + "/stdout"} // capture stray prints
	h.cmd.Stderr = &logWriter{name: h.manifest.Name}

	h.stdin = hostWrite
	h.rpcRead = hostRead
	h.stdout = bufio.NewScanner(hostRead)
	h.stdout.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)

	// Close child-side pipe ends after fork (cmd.Start duplicates them into child)
	extRead.Close()
	extWrite.Close()
	return nil
}

// Stop performs an ordered shutdown of the extension process:
//  1. Send shutdown notification (best-effort)
//  2. Close write pipe (host→ext) — extension gets EOF on FD 3
//  3. Give extension a moment to exit cleanly
//  4. Force-kill if it doesn't exit, then reap (cmd.Wait releases I/O goroutines)
//  5. readLoop sees EOF from closed write end → exits → signals readDone
//  6. Close read pipe after readLoop is done
func (h *Host) Stop() {
	h.closeOnce.Do(func() {
		name := h.manifest.Name
		slog.Debug("stop: begin", "ext", name)

		// Clean up event bus subscriptions
		h.subsMu.Lock()
		for id, unsub := range h.subscriptions {
			unsub()
			delete(h.subscriptions, id)
		}
		h.subsMu.Unlock()

		if h.cancel != nil {
			h.cancel()
		}

		// Best-effort shutdown notification
		_ = h.send(&Message{
			JSONRPC: "2.0",
			Method:  MethodShutdown,
		})

		// Close host→ext write pipe. Extension gets EOF on FD 3,
		// its scanner loop exits, and the process terminates cleanly.
		_ = h.stdin.Close()
		slog.Debug("stop: stdin closed", "ext", name)

		// Give the extension a moment to exit on its own.
		exited := make(chan struct{})
		go func() {
			_ = h.cmd.Wait()
			close(exited)
		}()

		killTimer := time.NewTimer(200 * time.Millisecond)
		defer killTimer.Stop()
		select {
		case <-exited:
			killTimer.Stop()
			slog.Debug("stop: clean exit", "ext", name)
		case <-killTimer.C:
			slog.Debug("stop: force kill", "ext", name)
			if h.cmd.Process != nil {
				_ = h.cmd.Process.Kill()
			}
			<-exited
			slog.Debug("stop: killed and reaped", "ext", name)
		}

		// Process is dead, its FD 4 is closed. readLoop's scanner
		// sees EOF and exits. Wait for it to finish.
		slog.Debug("stop: waiting for readDone", "ext", name)
		<-h.readDone
		slog.Debug("stop: readDone received", "ext", name)

		// readLoop is done — safe to close the read pipe.
		if h.rpcRead != nil {
			_ = h.rpcRead.Close()
		}
		close(h.closed)
		slog.Debug("stop: complete", "ext", name)
	})
}
