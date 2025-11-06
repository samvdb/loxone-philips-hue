package client

import (
	"context"
	"math"
	"time"
)

// Helpers

func boolTo01(b bool) int {
	if b {
		return 1
	}
	return 0
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Hue V2 lightlevel → lux, matching Python 10 ** (level/10000) - 1, safely clamped.
func luxFromHue(level int) int {
	x := float64(level) / 10000.0
	lux := math.Pow(10, x) - 1
	if lux < 0 {
		return 0
	}
	if lux > 100000 {
		return 100000
	}
	return int(lux + 0.5)
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
