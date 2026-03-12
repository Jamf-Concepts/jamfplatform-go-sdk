// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import "errors"

// Sentinel errors returned by the client.
var (
	ErrAuthentication = errors.New("jamfplatform: authentication failed")
	ErrNotFound       = errors.New("jamfplatform: resource not found")
)
