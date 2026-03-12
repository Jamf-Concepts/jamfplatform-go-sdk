// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"time"
)

// PollUntil repeatedly invokes checker until it reports completion or returns an error.
// Between attempts the function waits for the provided interval while respecting context cancellation.
func PollUntil(ctx context.Context, interval time.Duration, checker func(context.Context) (bool, error)) error {
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		done, err := checker(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
