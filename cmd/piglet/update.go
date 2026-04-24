package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func runUpdate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Printf("piglet %s\n", resolveVersion())

	cmd := exec.CommandContext(ctx, "go", "install", "github.com/dotcommander/piglet/cmd/piglet@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Printf("updated to latest\n")
	return nil
}
