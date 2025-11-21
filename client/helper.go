package client

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// Helpersvar
var (
	nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
)

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func cleanName(a string) string {
	a = strings.ToLower(a)
	a = nonAlnum.ReplaceAllString(a, "_")

	// trim multiple underscores
	return strings.Trim(a, "_")
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
