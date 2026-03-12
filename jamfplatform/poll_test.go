// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package jamfplatform

import (
	"context"
	"testing"
	"time"
)

func TestPollUntil_ExportedWrapper(t *testing.T) {
	calls := 0
	err := PollUntil(context.Background(), 10*time.Millisecond, func(_ context.Context) (bool, error) {
		calls++
		return calls >= 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}
