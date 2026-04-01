package ext

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dotcommander/piglet/core"
)

// CompactWithCircuitBreaker wraps a compact function with retry protection.
// After maxFails consecutive failures, compaction is disabled for cooldown duration.
func CompactWithCircuitBreaker(
	fn func(ctx context.Context, msgs []core.Message) ([]core.Message, error),
	maxFails int,
	cooldown time.Duration,
) func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
	var mu sync.Mutex
	var failCount int
	var cooldownUntil time.Time

	return func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		mu.Lock()
		if !cooldownUntil.IsZero() && time.Now().Before(cooldownUntil) {
			mu.Unlock()
			return nil, fmt.Errorf("compaction circuit breaker open until %s", cooldownUntil.Format(time.RFC3339))
		}
		mu.Unlock()

		result, err := fn(ctx, msgs)

		mu.Lock()
		defer mu.Unlock()
		if err != nil || len(result) == 0 {
			failCount++
			if failCount >= maxFails {
				cooldownUntil = time.Now().Add(cooldown)
			}
			return result, err
		}
		failCount = 0
		cooldownUntil = time.Time{}
		return result, nil
	}
}
