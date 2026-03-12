// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPollUntil_ImmediateSuccess(t *testing.T) {
	calls := 0
	err := PollUntil(context.Background(), 10*time.Millisecond, func(_ context.Context) (bool, error) {
		calls++
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestPollUntil_SuccessAfterRetries(t *testing.T) {
	calls := 0
	err := PollUntil(context.Background(), 10*time.Millisecond, func(_ context.Context) (bool, error) {
		calls++
		return calls >= 3, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestPollUntil_ErrorStopsPolling(t *testing.T) {
	calls := 0
	err := PollUntil(context.Background(), 10*time.Millisecond, func(_ context.Context) (bool, error) {
		calls++
		if calls == 2 {
			return false, fmt.Errorf("something broke")
		}
		return false, nil
	})
	if err == nil || err.Error() != "something broke" {
		t.Fatalf("err = %v, want 'something broke'", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestPollUntil_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := PollUntil(ctx, 10*time.Millisecond, func(_ context.Context) (bool, error) {
		return false, nil // never done
	})
	if err != context.DeadlineExceeded {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestPollUntil_ZeroIntervalDefaultsToOneSecond(t *testing.T) {
	// With zero interval, PollUntil should default to 1s.
	// We cancel quickly to verify it respects context.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	calls := 0
	err := PollUntil(ctx, 0, func(_ context.Context) (bool, error) {
		calls++
		return false, nil
	})
	if err != context.DeadlineExceeded {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	// With 1s default interval and 50ms timeout, should get at most 1 call
	if calls > 1 {
		t.Errorf("calls = %d, want <= 1 (1s default interval)", calls)
	}
}
