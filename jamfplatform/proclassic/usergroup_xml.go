// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// This file is NOT generated. It overrides the generated XML codec for
// UserGroup because the Jamf Classic server returns a polymorphic root
// element (<static_user_group> | <smart_user_group> | <user_group>)
// depending on the group's is_smart flag. The default struct-tag based
// Unmarshal rejects a root whose local name doesn't match the declared
// XMLName. MarshalXML forces the canonical <user_group> on the way out
// so writes always use the name the server accepts.

package proclassic

import "encoding/xml"

// UnmarshalXML accepts any root element name. Content is decoded into a
// locally-declared struct that mirrors the generated UserGroup fields
// sans its XMLName constraint, then copied back so the caller gets the
// same typed shape.
func (g *UserGroup) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var shadow struct {
		Criteria         *[]any      `xml:"criteria,omitempty"`
		ID               *int        `xml:"id,omitempty"`
		IsNotifyOnChange *bool       `xml:"is_notify_on_change,omitempty"`
		IsSmart          *bool       `xml:"is_smart,omitempty"`
		Name             *string     `xml:"name,omitempty"`
		Site             *SiteObject `xml:"site,omitempty"`
		Users            *[]any      `xml:"users,omitempty"`
	}
	if err := d.DecodeElement(&shadow, &start); err != nil {
		return err
	}
	*g = UserGroup{
		XMLName:          xml.Name{Local: start.Name.Local},
		Criteria:         shadow.Criteria,
		ID:               shadow.ID,
		IsNotifyOnChange: shadow.IsNotifyOnChange,
		IsSmart:          shadow.IsSmart,
		Name:             shadow.Name,
		Site:             shadow.Site,
		Users:            shadow.Users,
	}
	return nil
}

// MarshalXML always emits a <user_group> root regardless of what Unmarshal
// stored in XMLName, so request bodies match what the server accepts on
// create/update (the polymorphic read-side names are output-only).
func (g UserGroup) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "user_group"}
	type shadow struct {
		Criteria         *[]any      `xml:"criteria,omitempty"`
		ID               *int        `xml:"id,omitempty"`
		IsNotifyOnChange *bool       `xml:"is_notify_on_change,omitempty"`
		IsSmart          *bool       `xml:"is_smart,omitempty"`
		Name             *string     `xml:"name,omitempty"`
		Site             *SiteObject `xml:"site,omitempty"`
		Users            *[]any      `xml:"users,omitempty"`
	}
	s := shadow{
		Criteria:         g.Criteria,
		ID:               g.ID,
		IsNotifyOnChange: g.IsNotifyOnChange,
		IsSmart:          g.IsSmart,
		Name:             g.Name,
		Site:             g.Site,
		Users:            g.Users,
	}
	return e.EncodeElement(s, start)
}
