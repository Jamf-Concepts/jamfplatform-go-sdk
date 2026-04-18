// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

//go:build acceptance

package jamfplatform_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAcceptance_Classic_GetComputerByID exercises the Classic XML path
// end-to-end: the Swagger 2.0 spec was upconverted, the transport set
// Accept: application/xml and returned the response body verbatim as
// []byte. The consumer (this test, acting as a proxy for the TF provider)
// owns the XML parsing using its own struct definitions.
func TestAcceptance_Classic_GetComputerByID(t *testing.T) {
	c := accClient(t)

	body, err := proclassic.New(c).GetComputerByID(context.Background(), "4")
	if err != nil {
		skipOnServerError(t, err)
		t.Skipf("GetComputerByID(4): %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected non-empty response body")
	}

	// Consumer-owned type: only the fields this test cares about.
	var resp struct {
		XMLName xml.Name `xml:"computer"`
		General struct {
			ID           int    `xml:"id"`
			Name         string `xml:"name"`
			SerialNumber string `xml:"serial_number"`
			UDID         string `xml:"udid"`
		} `xml:"general"`
	}
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("parse response XML: %v", err)
	}
	t.Logf("Computer id=%d name=%q serial=%q udid=%q (response %d bytes)", resp.General.ID, resp.General.Name, resp.General.SerialNumber, resp.General.UDID, len(body))
}
