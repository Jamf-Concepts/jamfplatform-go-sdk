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

	"github.com/getkin/kin-openapi/openapi3"
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
		doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(root, spec.File))
		if err != nil {
			return fmt.Errorf("loading %s: %w", spec.File, err)
		}

		specFile := toSnakeCase(doc.Info.Title) + ".json"
		if spec.SpecFile != "" {
			specFile = spec.SpecFile
		}

		// Build whitelist of path+method pairs from config.
		type pathMethod struct{ path, method string }
		allowed := make(map[pathMethod]bool)
		for _, op := range spec.Operations {
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
