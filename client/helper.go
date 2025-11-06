package client

import (
	"context"
	"time"
)

// Helpers

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// sleepContext sleeps or returns early if ctx is cancelled.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
