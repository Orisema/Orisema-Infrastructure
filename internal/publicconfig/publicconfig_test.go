// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package publicconfig

import (
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

func TestMetadataIsDeterministicAndValueSensitive(t *testing.T) {
	a := []manifest.PublicEnvironmentEntry{{Name: "B", Value: "two"}, {Name: "A", Value: "one"}}
	b := []manifest.PublicEnvironmentEntry{{Name: "A", Value: "one"}, {Name: "B", Value: "two"}}
	namesA, hashA := Metadata(a)
	namesB, hashB := Metadata(b)
	if namesA != "A,B" || namesA != namesB || hashA != hashB || len(hashA) != 64 {
		t.Fatalf("unstable metadata: %q %q / %q %q", namesA, hashA, namesB, hashB)
	}
	b[1].Value = "changed"
	_, changed := Metadata(b)
	if changed == hashA {
		t.Fatal("value change did not change metadata")
	}
}

func TestEnvironmentMapUsesLastEntry(t *testing.T) {
	got := EnvironmentMap([]string{"IMAGE_DEFAULT=yes", "APP_MODE=old", "APP_MODE=laboratory", "malformed"})
	if got["APP_MODE"] != "laboratory" || got["IMAGE_DEFAULT"] != "yes" {
		t.Fatalf("EnvironmentMap() = %#v", got)
	}
}
