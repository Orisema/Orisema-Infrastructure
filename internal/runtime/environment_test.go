// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentRecognizesRootlessBoundary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "podman")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'true\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment, err := (Client{Binary: bin}).Environment(context.Background())
	if err != nil || !environment.Rootless {
		t.Fatalf("environment=%#v err=%v", environment, err)
	}
}

func TestEnvironmentFailsClosedOnUnknownOutput(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "podman")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'unknown\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (Client{Binary: bin}).Environment(context.Background()); err == nil {
		t.Fatal("expected unknown mode to fail closed")
	}
}

func TestStoragePathRequiresCleanAbsoluteGraphRoot(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "podman")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '/var/lib/containers/storage\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := (Client{Binary: bin}).StoragePath(context.Background())
	if err != nil || path != "/var/lib/containers/storage" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '../storage\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (Client{Binary: bin}).StoragePath(context.Background()); err == nil {
		t.Fatal("expected relative graph root to fail closed")
	}
}
