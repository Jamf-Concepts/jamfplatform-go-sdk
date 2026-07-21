// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// omap is an insertion-ordered JSON object. It exists because config.json is
// hand-authored and must round-trip byte-for-byte: Go's encoding/json sorts
// object keys alphabetically, which would rewrite all ~10k lines on every run.
// Decoding preserves the authored key order; encoding replays it, so a run that
// inserts nothing leaves the file untouched.
type omap struct {
	keys []string
	m    map[string]any
}

func newOmap() *omap { return &omap{m: map[string]any{}} }

func (o *omap) get(k string) (any, bool) { v, ok := o.m[k]; return v, ok }
func (o *omap) has(k string) bool        { _, ok := o.m[k]; return ok }

func (o *omap) set(k string, v any) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *omap) del(k string) {
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	for i, kk := range o.keys {
		if kk == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Typed accessors — return the zero value + false when the key is absent or the
// value is a different JSON type.
func (o *omap) str(k string) (string, bool) {
	v, ok := o.get(k)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func (o *omap) child(k string) (*omap, bool) {
	v, ok := o.get(k)
	if !ok {
		return nil, false
	}
	m, ok := v.(*omap)
	return m, ok
}

func (o *omap) slice(k string) ([]any, bool) {
	v, ok := o.get(k)
	if !ok {
		return nil, false
	}
	s, ok := v.([]any)
	return s, ok
}

// decodeOrdered parses JSON preserving object key order. Numbers decode as
// json.Number so integers and any decimals round-trip in their exact source
// form. Values are one of: *omap, []any, string, json.Number, bool, nil.
func decodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after top-level JSON value")
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return t, nil // scalar: string, json.Number, bool, nil
	}
	switch delim {
	case '{':
		o := newOmap()
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := kt.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string: %v", kt)
			}
			val, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			o.set(key, val)
		}
		if _, err := dec.Token(); err != nil { // consume '}'
			return nil, err
		}
		return o, nil
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil { // consume ']'
			return nil, err
		}
		return arr, nil
	}
	return nil, fmt.Errorf("unexpected delimiter %q", delim)
}

// encodeOrdered renders a decoded value to match Python's
// json.dump(indent=2, ensure_ascii=False) byte-for-byte (the format config.json
// is stored in), with a trailing newline.
func encodeOrdered(v any) []byte {
	var b bytes.Buffer
	writeValue(&b, v, 0)
	b.WriteByte('\n')
	return b.Bytes()
}

func writeIndent(b *bytes.Buffer, level int) {
	for range level {
		b.WriteString("  ")
	}
}

func writeValue(b *bytes.Buffer, v any, level int) {
	switch val := v.(type) {
	case *omap:
		if len(val.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, k := range val.keys {
			writeIndent(b, level+1)
			writeString(b, k)
			b.WriteString(": ")
			writeValue(b, val.m[k], level+1)
			if i < len(val.keys)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		writeIndent(b, level)
		b.WriteByte('}')
	case []any:
		if len(val) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, e := range val {
			writeIndent(b, level+1)
			writeValue(b, e, level+1)
			if i < len(val)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		writeIndent(b, level)
		b.WriteByte(']')
	case string:
		writeString(b, val)
	case json.Number:
		b.WriteString(string(val))
	case bool:
		if val {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case nil:
		b.WriteString("null")
	default:
		panic(fmt.Sprintf("encodeOrdered: unexpected type %T", v))
	}
}

// writeString escapes a string the way Python's json encoder does with
// ensure_ascii=False: backslash, quote and the five named control escapes get
// short forms, other C0 control chars get \u00xx (lowercase), and everything
// else — including <, >, & and non-ASCII — is written raw.
func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
