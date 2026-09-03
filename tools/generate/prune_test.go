// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// resetEmitted clears the package-level write log between subtests. The log is
// global because every emitter writes through one function; the tests have to
// undo that or they contaminate each other.
func resetEmitted(t *testing.T) {
	t.Helper()
	emitted = map[string]bool{}
	t.Cleanup(func() { emitted = map[string]bool{} })
}

// write creates a file and returns its absolute path. marked controls whether it
// carries the generated-file marker.
func write(t *testing.T, dir, name string, marked bool) string {
	t.Helper()
	body := "package p\n"
	if marked {
		body = generatedMarker + "\n\npackage p\n"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func TestPruneStale(t *testing.T) {
	t.Run("deletes a marked file the run did not write", func(t *testing.T) {
		resetEmitted(t)
		root := t.TempDir()
		dir := filepath.Join(root, "pkg")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		fresh := write(t, dir, "kept.go", true)
		stale := write(t, dir, "stale.go", true)
		emitted[fresh] = true

		if err := pruneStale(root); err != nil {
			t.Fatalf("pruneStale: %v", err)
		}
		if !exists(t, fresh) {
			t.Error("deleted a file the run wrote")
		}
		if exists(t, stale) {
			t.Error("stale generated file survived — this is the v1872 failure the prune exists to stop")
		}
	})

	t.Run("never deletes a handwritten file", func(t *testing.T) {
		resetEmitted(t)
		root := t.TempDir()
		dir := filepath.Join(root, "pkg")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		fresh := write(t, dir, "generated.go", true)
		// The real cases: jamfplatform/client.go and the acc_*_test.go suite
		// live in a directory the generator writes into and must survive.
		hand := write(t, dir, "client.go", false)
		acc := write(t, dir, "acc_thing_test.go", false)
		emitted[fresh] = true

		if err := pruneStale(root); err != nil {
			t.Fatalf("pruneStale: %v", err)
		}
		if !exists(t, hand) || !exists(t, acc) {
			t.Fatal("deleted a handwritten file: the marker guard is not holding")
		}
	})

	t.Run("prunes nothing when the run wrote nothing", func(t *testing.T) {
		resetEmitted(t)
		root := t.TempDir()
		dir := filepath.Join(root, "pkg")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		orphan := write(t, dir, "generated.go", true)

		if err := pruneStale(root); err != nil {
			t.Fatalf("pruneStale: %v", err)
		}
		if !exists(t, orphan) {
			t.Fatal("emptied a directory on a run that emitted nothing — a failed generate would wipe the tree")
		}
	})

	t.Run("ignores directories the run did not write into", func(t *testing.T) {
		resetEmitted(t)
		root := t.TempDir()
		written := filepath.Join(root, "written")
		untouched := filepath.Join(root, "untouched")
		for _, d := range []string{written, untouched} {
			if err := os.Mkdir(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		fresh := write(t, written, "a.go", true)
		elsewhere := write(t, untouched, "b.go", true)
		emitted[fresh] = true

		if err := pruneStale(root); err != nil {
			t.Fatalf("pruneStale: %v", err)
		}
		if !exists(t, elsewhere) {
			t.Fatal("pruned a directory the generator never wrote to")
		}
	})

	t.Run("leaves non-Go files alone", func(t *testing.T) {
		resetEmitted(t)
		root := t.TempDir()
		dir := filepath.Join(root, "pkg")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		fresh := write(t, dir, "a.go", true)
		emitted[fresh] = true
		other := filepath.Join(dir, "notes.md")
		if err := os.WriteFile(other, []byte(generatedMarker), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := pruneStale(root); err != nil {
			t.Fatalf("pruneStale: %v", err)
		}
		if !exists(t, other) {
			t.Fatal("pruned a non-Go file")
		}
	})
}

func TestPruneStaleSpecs(t *testing.T) {
	t.Run("deletes a spec the run did not write", func(t *testing.T) {
		resetEmitted(t)
		root := t.TempDir()
		dir := filepath.Join(root, "api")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		fresh := write(t, dir, "kept.json", false)
		stale := write(t, dir, "stale.json", false)
		emitted[fresh] = true

		if err := pruneStaleSpecs(root, "api"); err != nil {
			t.Fatalf("pruneStaleSpecs: %v", err)
		}
		if !exists(t, fresh) {
			t.Error("deleted a spec the run published")
		}
		if exists(t, stale) {
			t.Error("stale published spec survived")
		}
	})

	t.Run("prunes nothing when no spec was published", func(t *testing.T) {
		resetEmitted(t)
		root := t.TempDir()
		dir := filepath.Join(root, "api")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// The CI shape: source specs absent, publishing skipped, api/ is the
		// generator's own input. Deleting here would destroy it.
		spec := write(t, dir, "pro_api.json", false)

		if err := pruneStaleSpecs(root, "api"); err != nil {
			t.Fatalf("pruneStaleSpecs: %v", err)
		}
		if !exists(t, spec) {
			t.Fatal("deleted a published spec on a run that published none — this would break the CI fallback")
		}
	})

	t.Run("tolerates a missing spec dir", func(t *testing.T) {
		resetEmitted(t)
		if err := pruneStaleSpecs(t.TempDir(), "api"); err != nil {
			t.Fatalf("pruneStaleSpecs: %v", err)
		}
	})
}

func TestWriteGeneratedRecordsPath(t *testing.T) {
	resetEmitted(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	if err := writeGenerated(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatalf("writeGenerated: %v", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if !emitted[abs] {
		t.Fatal("writeGenerated did not record the path — the file would be pruned on the next run")
	}
}

// TestHasGeneratedMarker pins the discriminator itself, since every deletion
// depends on it.
func TestHasGeneratedMarker(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"generated", generatedMarker + "\n\npackage p\n", true},
		{"handwritten", "// Copyright Jamf Software LLC 2026\n\npackage p\n", false},
		{"empty", "", false},
		{"marker not first", "package p\n" + generatedMarker + "\n", false},
		{"truncated marker", generatedMarker[:20], false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".go")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := hasGeneratedMarker(path)
			if err != nil {
				t.Fatalf("hasGeneratedMarker: %v", err)
			}
			if got != tc.want {
				t.Errorf("hasGeneratedMarker(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
