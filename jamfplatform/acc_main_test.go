// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupSmartGroupFixture()
	os.Exit(code)
}
