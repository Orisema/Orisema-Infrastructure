// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package buildinfo

import (
	"runtime"
	"testing"
)

func TestCurrentUsesDevelopmentDefaults(t *testing.T) {
	got := Current()
	if got.Version != Development || got.Commit != "unknown" || got.Built != "unknown" {
		t.Fatalf("unexpected development metadata: %#v", got)
	}
	if got.Go != runtime.Version() || got.OS != runtime.GOOS || got.Arch != runtime.GOARCH {
		t.Fatalf("unexpected runtime metadata: %#v", got)
	}
}

func TestNewPreservesInjectedMetadata(t *testing.T) {
	got := New("v0.1.0-alpha.1", "abc1234", "2026-10-27T18:00:00Z", "go1.23.9", "linux", "arm64")
	want := "Orisema Infrastructure\nVersion: v0.1.0-alpha.1\nCommit: abc1234\nBuilt: 2026-10-27T18:00:00Z\nGo: go1.23.9\nOS/Arch: linux/arm64\n"
	if got.String() != want {
		t.Fatalf("String()=%q want=%q", got.String(), want)
	}
}

func TestNewNormalizesMissingMetadata(t *testing.T) {
	got := New(" ", "", "", "", "", "")
	if got.Version != Development || got.Commit != "unknown" || got.Built != "unknown" || got.Go != "unknown" || got.OS != "unknown" || got.Arch != "unknown" {
		t.Fatalf("unexpected fallbacks: %#v", got)
	}
}
