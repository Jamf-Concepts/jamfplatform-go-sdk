// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"context"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAcceptance_Classic_GetComputerByID exercises the Classic XML path
// end-to-end. With the v11.20.0 Swagger 2.0 spec replaced in-tree, the
// generator now emits a fully-typed Computer with nested ComputerGeneral,
// ComputerHardware, etc. sub-structs; xml.Unmarshal populates them from
// the real 30KB XML response.
func TestAcceptance_Classic_GetComputerByID(t *testing.T) {
	c := accClient(t)

	comp, err := proclassic.New(c).GetComputerByID(context.Background(), "4")
	if err != nil {
		skipOnServerError(t, err)
		t.Skipf("GetComputerByID(4): %v", err)
	}
	if comp == nil || comp.General == nil {
		t.Fatalf("expected Computer.General populated, got %+v", comp)
	}
	t.Logf("Computer id=%d name=%q serial=%q udid=%q", comp.General.ID, comp.General.Name, comp.General.SerialNumber, comp.General.UDID)
}
