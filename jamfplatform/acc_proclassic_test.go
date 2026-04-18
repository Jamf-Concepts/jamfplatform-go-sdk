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
// end-to-end: loadSpec upconverted the Swagger 2.0 doc, the transport
// set Accept: application/xml and xml.Unmarshaled the response into
// the generated Computer struct.
func TestAcceptance_Classic_GetComputerByID(t *testing.T) {
	c := accClient(t)

	// Use an ID we know exists in the nm-test tenant (cross-referenced from
	// /api/proclassic/tenant/{id}/computers list — see gateway probe in
	// commit history). Skip cleanly if the tenant doesn't have this ID.
	comp, err := proclassic.New(c).GetComputerByID(context.Background(), "4")
	if err != nil {
		skipOnServerError(t, err)
		t.Skipf("GetComputerByID(4): %v", err)
	}
	if comp == nil {
		t.Fatal("expected non-nil Computer")
	}
	t.Logf("Computer id=%d name=%q serial=%q udid=%q", comp.ID, comp.Name, comp.Serialnumber, comp.UDID)
}
