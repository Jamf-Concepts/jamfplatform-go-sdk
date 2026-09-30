// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GitOps v2267 prefixed 22 jpapi and 4 capi operation summaries with
// "Deprecated - ". The summary is what becomes a method's godoc sentence, so
// without the strip every one of those methods read
// "GetActivationCode deprecated - finds the Jamf Pro activation code." — a
// broken doc comment, and exactly the defect v2192's "Preview - " prefix
// caused on the ai-governance package.
func TestStripDeprecatedPrefixOnlyFiresWhenTheOperationDeclaresDeprecated(t *testing.T) {
	for _, tc := range []struct {
		name       string
		summary    string
		deprecated bool
		want       string
	}{
		{"hyphen", "Deprecated - Finds the Jamf Pro activation code", true, "Finds the Jamf Pro activation code"},
		{"colon", "Deprecated: Get information about mdm commands made by Jamf Pro.", true, "Get information about mdm commands made by Jamf Pro."},
		{"en dash", "Deprecated – Remove attachment", true, "Remove attachment"},
		{"em dash", "Deprecated — Remove attachment", true, "Remove attachment"},
		{"no space", "Deprecated-Remove attachment", true, "Remove attachment"},
		{"leading space", "  Deprecated - Remove attachment", true, "Remove attachment"},
		{"unprefixed summary is untouched", "Return paginated Computer Inventory records", true, "Return paginated Computer Inventory records"},

		// The gate is `deprecated: true`, not the text, so an operation the
		// spec never marked deprecated keeps its prose even when it opens with
		// the word.
		{"not declared deprecated", "Deprecated - Finds the activation code", false, "Deprecated - Finds the activation code"},
		{"the word as prose on a live op", "Deprecated fields are omitted from this report", false, "Deprecated fields are omitted from this report"},

		// The prefix form requires a separator, so prose that merely begins
		// with the word survives on a deprecated operation too.
		{"the word as prose on a deprecated op", "Deprecated fields are omitted from this report", true, "Deprecated fields are omitted from this report"},

		// The 20 pre-existing capi summaries carry the state MID-SENTENCE.
		// Those predate the prefix convention, render as valid godoc, and must
		// not be touched — only a leading prefix breaks the sentence.
		{"inline parenthetical is left alone", `Finds all patches (Deprecated - Please transition use to Jamf Pro API endpoint "/v2/patch-software-title-configurations".`, true, `Finds all patches (Deprecated - Please transition use to Jamf Pro API endpoint "/v2/patch-software-title-configurations".`},
		{"inline bare parenthetical is left alone", "Returns basic information about Jamf Pro (Deprecated)", true, "Returns basic information about Jamf Pro (Deprecated)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripDeprecatedPrefix(tc.summary, tc.deprecated); got != tc.want {
				t.Fatalf("stripDeprecatedPrefix(%q, %v) = %q, want %q", tc.summary, tc.deprecated, got, tc.want)
			}
		})
	}
}

// The two strips compose: an operation could in principle be declared both
// preview and deprecated, and the order they are applied in must not leave
// either prefix behind.
func TestBothStatePrefixesComeOffTogether(t *testing.T) {
	got := stripDeprecatedPrefix(stripPreviewPrefix("Preview - Deprecated - Get a thing", true), true)
	if got != "Get a thing" {
		t.Fatalf("composed strip = %q, want %q", got, "Get a thing")
	}
	got = stripDeprecatedPrefix(stripPreviewPrefix("Deprecated - Get a thing", true), true)
	if got != "Get a thing" {
		t.Fatalf("composed strip (deprecated only) = %q, want %q", got, "Get a thing")
	}
}

// The regression this guards is the one the generated tree would have shipped:
// a godoc sentence whose verb phrase is the word "deprecated". No generated
// method comment may contain it, in any package — the deprecation state is a
// separate "Deprecated:" paragraph, which staticcheck's SA1019 reads and
// go/doc renders, and which this assertion deliberately does not match
// because it is checked case-sensitively on the lowercased prefix form
// lowerFirst would produce.
func TestNoGeneratedMethodCommentOpensWithTheDeprecatedState(t *testing.T) {
	root := filepath.Join("..", "..", "jamfplatform")
	var offenders []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(b), "\n") {
			if !strings.HasPrefix(line, "// ") {
				continue
			}
			// The shape to catch is "// MethodName deprecated - …" or
			// "// MethodName preview - …": lowerFirst has lowercased a state
			// prefix into the method's own verb phrase.
			for _, bad := range []string{" deprecated - ", " deprecated: ", " preview - ", " preview: "} {
				if strings.Contains(line, bad) {
					offenders = append(offenders, path+": "+line)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(offenders) > 0 {
		t.Fatalf("generated godoc carries a state prefix inside the method's verb phrase — "+
			"the spec prefixed the operation summary and the strip did not fire:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
