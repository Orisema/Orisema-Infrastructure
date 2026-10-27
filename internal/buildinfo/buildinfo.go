// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package buildinfo exposes metadata embedded in the Orisema binary.
package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
)

const Development = "development"

// These values are intentionally unexported. Release builds replace them with
// -ldflags -X; ordinary go builds retain the explicit development defaults.
var (
	version = Development
	commit  = "unknown"
	built   = "unknown"
)

// Info is the immutable build description printed by the CLI.
type Info struct {
	Version string
	Commit  string
	Built   string
	Go      string
	OS      string
	Arch    string
}

// Current returns metadata for the running binary.
func Current() Info {
	return New(version, commit, built, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// New normalizes supplied metadata and is kept deterministic for testing.
func New(version, commit, built, goVersion, goos, goarch string) Info {
	return Info{
		Version: fallback(version, Development),
		Commit:  fallback(commit, "unknown"),
		Built:   fallback(built, "unknown"),
		Go:      fallback(goVersion, "unknown"),
		OS:      fallback(goos, "unknown"),
		Arch:    fallback(goarch, "unknown"),
	}
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

// String renders the stable, human-readable version report.
func (info Info) String() string {
	return fmt.Sprintf("Orisema Infrastructure\nVersion: %s\nCommit: %s\nBuilt: %s\nGo: %s\nOS/Arch: %s/%s\n",
		info.Version, info.Commit, info.Built, info.Go, info.OS, info.Arch)
}
