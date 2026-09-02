// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Post-emit validator
// ---------------------------------------------------------------------------
//
// The generator has several silent-drop paths: a $ref pointing at a schema
// the walker doesn't hoist, an array/scalar top-level schema skipped by
// extractTypes, a config-level responseType override pointing at a schema
// name that doesn't exist. Any of these produce methods that reference a
// Go type that was never emitted — the failure surfaces later as a go
// build error with no hint that the generator was to blame.
//
// validateTypeReferences walks every method's declared type references
// and errors out when any reference does not resolve to either a declared
// package type or a Go builtin we intentionally produce. The check is
// pattern-based: it asks "did the generator actually emit a definition
// for every name the templates will render?" — catching the whole class
// of silent-drop bugs at generate time instead of at go build.

// builtin lists Go type expressions the templates may synthesize directly
// (scalars, pre-declared aliases, and the stdlib types extractTypes /
// schemaRefToGoType emit). References to these need not appear in the
// per-package declared set.
var builtinTypeRefs = map[string]bool{
	"any":             true,
	"bool":            true,
	"byte":            true,
	"float32":         true,
	"float64":         true,
	"int":             true,
	"int32":           true,
	"int64":           true,
	"string":          true,
	"time.Time":       true,
	"json.RawMessage": true,
	"xml.Name":        true,
	// BigInt, NotificationValue, and PayloadsXMLText are supplemental types
	// the generator emits into XML packages (see xml_helpers.go) as
	// FieldTypeOverride targets for spec/wire bugs (int overflow, repeated
	// duplicate elements, single-escape chardata for Classic <payloads>).
	// Treat them as builtins for validation — the validator can't see
	// supplemental files and would false-positive.
	"BigInt":            true,
	"NotificationValue": true,
	"PayloadsXMLText":   true,
}

// validateTypeReferences reports missing Go types referenced by methods.
// declared is the union of type names the generator emitted in this
// package; pkgContext is used for error messages.
func validateTypeReferences(pkgContext string, declared []GoType, methods []GoMethod) error {
	declaredSet := map[string]bool{}
	for _, t := range declared {
		declaredSet[t.Name] = true
	}

	var missing []string
	note := func(ref, methodName, kind string) {
		if ref == "" {
			return
		}
		name := normalizeTypeRef(ref)
		if name == "" || builtinTypeRefs[name] || declaredSet[name] {
			return
		}
		missing = append(missing, fmt.Sprintf("  - method %s (%s): Go type %q not emitted", methodName, kind, name))
	}

	for _, m := range methods {
		note(m.RequestType, m.Name, "request")
		note(m.ResponseType, m.Name, "response")
		note(m.ItemType, m.Name, "paginated item")
		note(m.UnwrapResults, m.Name, "unwrapped results")
	}

	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %d unresolved type reference(s):\n%s\n\nfix: either add the schema under components/schemas so extractTypes emits it, or correct the config-level requestType/responseType override", pkgContext, len(missing), strings.Join(missing, "\n"))
}

// validateUnwrapCodec rejects `unwrapResults` on an operation whose response
// will not be decoded as JSON.
//
// The unwrap template generates a call to client.UnwrapResults, which reads the
// first non-space byte of the body and json.Unmarshals it. Two things select a
// different codec at runtime and **neither looks at the Go type**: Transport.Do
// routes any path containing "/proclassic/" through xml.Unmarshal, and a spec
// declared `"format": "xml"` generates XML struct tags throughout. Either way
// the bytes handed to UnwrapResults are XML, the shape sniff sees `<`, and the
// caller gets a decode error on every call.
//
// This has no users to break — all six unwrap operations are JSON — and the
// pre-existing struct decode was equally wrong here, so this is not a
// regression being papered over. It is the failure mode being made
// unreachable, because the config key gives no hint that it is JSON-only and
// the cost of finding out is a method that compiles, generates a passing
// httptest stub, and fails against the server.
func validateUnwrapCodec(pkgContext string, methods []GoMethod) error {
	var bad []string
	for _, m := range methods {
		if m.UnwrapResults == "" {
			continue
		}
		switch {
		case strings.EqualFold(m.Format, "xml"):
			bad = append(bad, fmt.Sprintf("  - method %s: spec declares \"format\": \"xml\"", m.Name))
		case m.Namespace == "proclassic":
			bad = append(bad, fmt.Sprintf("  - method %s: namespace %q is decoded as XML by the transport", m.Name, m.Namespace))
		}
	}

	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %d operation(s) declare unwrapResults on a non-JSON response:\n%s\n\nfix: unwrapResults is JSON-only. Use responseType with an XML-tagged schema, or rawBody, for a Classic or format=xml operation", pkgContext, len(bad), strings.Join(bad, "\n"))
}

// validateNoUntypedFields rejects an emitted struct field whose Go type is a
// bare `any` or `*any`.
//
// That is the tell of a schema construct the generator has no branch for.
// schemaRefToGoType ends in `default: return "any"`, so a property whose
// schema it does not recognise produces a field that type-checks nothing,
// marshals whatever it is handed, and — the transport setting no
// DisallowUnknownFields — decodes anything without complaint. Nothing fails,
// at generate time or at run time, and the consumer discovers it by having to
// reimplement the shape by hand.
//
// The live case was account-sso's ConnectionRequest.connection: a
// discriminator-less `oneOf` over four `$ref`s, reached through a property, so
// never hoisted and never named. It carried the whole provider-settings body
// of every SSO connection create and update as `any`, while all four settings
// schemas sat in types.go referenced by no signature in the module. isRefUnion
// gives that shape a type now; this check is what stops the next unrecognised
// construct reaching a consumer the same way.
//
// Container `any`s are deliberately allowed. `map[string]any` and `[]any` are
// what a genuinely freeform or unconstrained-item schema *should* produce, and
// they say so at the call site.
func validateNoUntypedFields(pkgContext string, declared []GoType) error {
	var bad []string
	for _, t := range declared {
		for _, f := range t.Fields {
			if f.Type == "any" || f.Type == "*any" {
				bad = append(bad, fmt.Sprintf("  - %s.%s (json:%q) is %s", t.Name, f.Name, f.JSONTag, f.Type))
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %d field(s) emitted as untyped any:\n%s\n\nfix: teach the generator the schema construct behind the property (see isRefUnion for the discriminator-less oneOf case), or correct the property's schema through config. Do not silence this by widening the check — an `any` field here is a shape no caller can rely on", pkgContext, len(bad), strings.Join(bad, "\n"))
}

// normalizeTypeRef strips slice / pointer prefixes and returns the bare
// Go type name the validator should look up. Composite expressions like
// `map[string]Foo` yield "Foo" — we care about the user-defined type at
// the leaf, not the container syntax.
func normalizeTypeRef(ref string) string {
	for {
		trimmed := strings.TrimPrefix(ref, "*")
		trimmed = strings.TrimPrefix(trimmed, "[]")
		if strings.HasPrefix(trimmed, "map[") {
			if i := strings.Index(trimmed, "]"); i >= 0 {
				trimmed = trimmed[i+1:]
			}
		}
		if trimmed == ref {
			return ref
		}
		ref = trimmed
	}
}
