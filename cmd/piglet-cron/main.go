package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dotcommander/piglet/extensions/cron"
)

func main() {
	var (
		verbose  bool
		taskName string
	)

	args := os.Args[1:]
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(os.Stderr, "Usage: piglet-cron run [--verbose] [--task NAME]")
		os.Exit(1)
	}
	args = args[1:] // skip "run"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--verbose", "-v":
			verbose = true
		case "--task":
			if i+1 < len(args) {
				i++
				taskName = args[i]
			} else {
				fmt.Fprintln(os.Stderr, "--task requires a name")
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	level := slog.LevelWarn
	if verbose {
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	lock, err := cron.Acquire()
	if err != nil {
		slog.Warn("lock", "error", err)
		os.Exit(0) // Not an error — another instance is running.
	}
	defer lock.Release()

	opts := cron.RunOptions{
		Verbose:  verbose,
		TaskName: taskName,
		Force:    taskName != "", // --task implies force
	}

	if err := cron.Run(ctx, opts); err != nil {
		slog.Error("run", "error", err)
		os.Exit(1)
	}
}
