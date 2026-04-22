// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Publish filtered specs
// ---------------------------------------------------------------------------

func publishSpecs(root string, cfg Config) error {
	outDir := filepath.Join(root, cfg.SpecDir)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating spec dir: %w", err)
	}

	for _, spec := range cfg.Specs {
		// Types-only specs are internal service schemas not part of the
		// published API surface — skip them.
		if spec.TypesOnly {
			continue
		}
		doc, err := loadSpec(filepath.Join(root, spec.File), allowedOpsSet(spec))
		if err != nil {
			return fmt.Errorf("loading %s: %w", spec.File, err)
		}

		specFile := toSnakeCase(doc.Info.Title) + ".json"
		if spec.SpecFile != "" {
			specFile = spec.SpecFile
		}

		// Apply the same skipDeprecated filter the code generator uses so
		// the published spec describes only the SDK's real surface — a
		// spec op removed from generated Go for being deprecated must not
		// linger in api/*.json.
		publishOps := spec.Operations
		if spec.SkipDeprecated {
			publishOps = dropDeprecatedOps(doc, spec)
		}

		// Build whitelist of path+method pairs from config.
		type pathMethod struct{ path, method string }
		allowed := make(map[pathMethod]bool)
		for _, op := range publishOps {
			method, path := op.parseOp()
			allowed[pathMethod{path, method}] = true
		}

		// Filter paths: remove operations not in whitelist, remove empty path items.
		for _, path := range doc.Paths.InMatchingOrder() {
			item := doc.Paths.Find(path)
			if item == nil {
				continue
			}
			for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
				if item.GetOperation(method) != nil && !allowed[pathMethod{path, method}] {
					switch method {
					case "GET":
						item.Get = nil
					case "POST":
						item.Post = nil
					case "PUT":
						item.Put = nil
					case "PATCH":
						item.Patch = nil
					case "DELETE":
						item.Delete = nil
					}
				}
			}
			// Remove path entirely if no operations remain.
			hasOps := item.Get != nil || item.Post != nil || item.Put != nil ||
				item.Patch != nil || item.Delete != nil
			if !hasOps {
				doc.Paths.Delete(path)
			}
		}

		// Collect all $ref'd schemas from remaining operations.
		usedSchemas := make(map[string]bool)
		collectRefs(doc, usedSchemas)

		// Preserve schemas named by config-level requestType/responseType
		// overrides and walk them transitively. These are *not* reachable
		// via $ref from the spec itself (Classic's operations carry no
		// typed request bodies at all — the names come from config), so
		// collectRefs misses them and they'd otherwise be pruned.
		// Without this the published spec drops every *_post schema and
		// downstream generation hits "Go type not emitted" errors.
		if doc.Components != nil && doc.Components.Schemas != nil {
			walk := newSchemaWalker(doc, func(name string) bool {
				if usedSchemas[name] {
					return false
				}
				usedSchemas[name] = true
				return true
			})
			for _, op := range publishOps {
				for _, typeName := range []string{op.RequestType, op.ResponseType} {
					if typeName == "" {
						continue
					}
					usedSchemas[typeName] = true
					if ref, ok := doc.Components.Schemas[typeName]; ok {
						walk(ref)
					}
				}
			}
		}

		// Prune unreferenced schemas.
		if doc.Components != nil && doc.Components.Schemas != nil {
			for name := range doc.Components.Schemas {
				if !usedSchemas[name] {
					delete(doc.Components.Schemas, name)
				}
			}
		}

		// Remove internal paths (e.g. /internal/v1/...).
		for _, path := range doc.Paths.InMatchingOrder() {
			if strings.HasPrefix(path, "/internal/") {
				doc.Paths.Delete(path)
			}
		}

		// Marshal to JSON.
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling %s: %w", specFile, err)
		}

		outPath := filepath.Join(outDir, specFile)
		if err := os.WriteFile(outPath, append(data, '\n'), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		log.Printf("wrote %s/%s", cfg.SpecDir, specFile)
	}
	return nil
}
