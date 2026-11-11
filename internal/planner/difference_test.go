// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"strings"
	"testing"
)

func TestClassifyDifferencesIsClosedCanonicalAndValueFree(t *testing.T) {
	canary := "secret-canary-must-not-appear"
	details := []string{
		"network attachment differs",
		"configured image differs",
		"secret binding metadata differs",
		"public configuration differs",
	}
	got := classifyDifferences(details)
	if len(got) != 4 {
		t.Fatalf("differences=%#v", got)
	}
	want := []DifferenceKind{DifferenceImage, DifferenceNetworkBinding, DifferencePublicConfig, DifferenceSecretBinding}
	for i := range want {
		if got[i].Kind != want[i] {
			t.Fatalf("differences=%#v; kind[%d]=%s want %s", got, i, got[i].Kind, want[i])
		}
	}
	encoded := DifferenceFingerprint(got)
	if len(encoded) != 64 || strings.Contains(encoded, canary) {
		t.Fatalf("unsafe fingerprint %q", encoded)
	}
	reversed := []Difference{got[3], got[2], got[1], got[0]}
	if DifferenceFingerprint(reversed) != encoded {
		t.Fatal("fingerprint depends on input order")
	}
}

func TestClassifyDifferencesKeepsUnknownFailClosed(t *testing.T) {
	const canary = "future-secret-canary"
	got := classifyDifferences([]string{canary})
	if len(got) != 1 || got[0].Kind != DifferenceUnknown || strings.Contains(got[0].Detail, canary) {
		t.Fatalf("differences=%#v; want UNKNOWN", got)
	}
}
