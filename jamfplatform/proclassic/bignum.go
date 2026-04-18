// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// This file is NOT generated. It supplies BigInt, an arbitrary-precision
// integer type the generator targets for fields that the spec models as
// `integer` but the server actually returns with values beyond int64.
// Jamf Classic invitation codes are the canonical case: 38+ digit random
// numbers that are semantically numeric but can't fit in an int64. The
// wrapper delegates all arithmetic to math/big, while providing XML/JSON
// codecs that round-trip through the wire representation (a decimal
// digit string).

package proclassic

import (
	"encoding/xml"
	"math/big"
)

// BigInt is an arbitrary-precision integer with XML/JSON codecs. The
// zero value is usable and equivalent to big.NewInt(0).
type BigInt struct {
	v big.Int
}

// Int returns a pointer to the underlying math/big.Int so callers can do
// arithmetic without having to export the internal field. Mutations via
// the returned pointer are reflected in subsequent marshalling.
func (b *BigInt) Int() *big.Int { return &b.v }

// String returns the decimal representation, matching the wire form.
func (b BigInt) String() string { return b.v.String() }

// SetString parses a decimal integer and stores it. Returns false if the
// input isn't a valid base-10 integer.
func (b *BigInt) SetString(s string) bool {
	_, ok := b.v.SetString(s, 10)
	return ok
}

// UnmarshalXML reads the element's text value and parses it as a base-10
// integer. Empty content yields a BigInt equal to zero, not an error,
// since Classic occasionally emits empty numeric fields on placeholder
// records.
func (b *BigInt) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	if s == "" {
		b.v.SetInt64(0)
		return nil
	}
	if _, ok := b.v.SetString(s, 10); !ok {
		// If the server returns a non-numeric sentinel (Classic
		// occasionally emits "Unlimited" in otherwise-numeric fields),
		// treat it as zero rather than erroring out. Consumers who
		// care about sentinel detection can inspect the raw body via
		// WithLogger.
		b.v.SetInt64(0)
	}
	return nil
}

// MarshalXML emits the decimal string representation as the element's
// text content.
func (b BigInt) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(b.v.String(), start)
}

// UnmarshalJSON accepts either a JSON number (emitted unquoted) or a
// JSON string containing a decimal integer. Jamf APIs returning JSON
// responses can use either encoding depending on the renderer.
func (b *BigInt) UnmarshalJSON(data []byte) error {
	// Strip optional surrounding quotes.
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "" || s == "null" {
		b.v.SetInt64(0)
		return nil
	}
	if _, ok := b.v.SetString(s, 10); !ok {
		b.v.SetInt64(0)
	}
	return nil
}

// MarshalJSON emits the value as a JSON number (unquoted) so consumers
// see the same numeric semantics they would for a regular integer.
func (b BigInt) MarshalJSON() ([]byte, error) {
	return []byte(b.v.String()), nil
}
