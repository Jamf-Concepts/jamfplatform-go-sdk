// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

// Command generate reads OpenAPI spec files and produces typed Go SDK methods
// and unit tests that call into the internal/client transport layer.
//
// Usage:
//
//	go run ./tools/generate [flags]
//	  -config  path to config.json  (default: tools/generate/config.json)
//	  -root    repo root directory   (default: auto-detected from git)
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	configPath := flag.String("config", "", "path to config.json")
	rootDir := flag.String("root", "", "repo root directory")
	flag.Parse()

	if *rootDir == "" {
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			log.Fatal("cannot detect repo root: ", err)
		}
		*rootDir = strings.TrimSpace(string(out))
	}
	if *configPath == "" {
		*configPath = filepath.Join(*rootDir, "tools", "generate", "config.json")
	}

	data, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("reading config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("parsing config: %v", err)
	}

	if err := validateConfig(cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	emittedTypes := make(map[string]bool) // dedup types across root-package specs
	pkgBuckets := make(map[string][]loadedSpec)
	hasSourceSpecs := true
	for _, spec := range cfg.Specs {
		specPath, usedFallback, err := resolveSpecPath(*rootDir, cfg, spec)
		if err != nil {
			log.Fatalf("spec %s: %v", spec.File, err)
		}
		if usedFallback {
			hasSourceSpecs = false
		}
		if spec.Package == "" {
			if err := processSpec(*rootDir, cfg, spec, specPath, emittedTypes); err != nil {
				log.Fatalf("spec %s: %v", spec.File, err)
			}
		} else {
			pkgBuckets[spec.Package] = append(pkgBuckets[spec.Package], loadedSpec{spec: spec, specPath: specPath})
		}
	}

	pkgNames := make([]string, 0, len(pkgBuckets))
	for name := range pkgBuckets {
		pkgNames = append(pkgNames, name)
	}
	sort.Strings(pkgNames)
	for _, pkgName := range pkgNames {
		if err := processPackage(*rootDir, cfg, pkgName, pkgBuckets[pkgName]); err != nil {
			log.Fatalf("package %s: %v", pkgName, err)
		}
	}
	if err := writeStaticFiles(*rootDir, cfg); err != nil {
		log.Fatalf("static files: %v", err)
	}
	// Only publish filtered specs when source specs are available.
	// In CI the source specs are private; the generator reads from the
	// already-published api/ specs and only regenerates Go code.
	if cfg.SpecDir != "" && hasSourceSpecs {
		if err := publishSpecs(*rootDir, cfg); err != nil {
			log.Fatalf("publishing specs: %v", err)
		}
	}
	log.Println("generation complete")
}
