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
	return fmt.Errorf("%s: %d unresolved type reference(s):\n%s\n\nFix: either add the schema under components/schemas so extractTypes emits it, or correct the config-level requestType/responseType override.", pkgContext, len(missing), strings.Join(missing, "\n"))
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
