// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package storagecheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAvailableAcceptsRealDirectoryAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	available, err := Available(dir)
	if err != nil || available == 0 {
		t.Fatalf("available=%d err=%v", available, err)
	}
	link := filepath.Join(t.TempDir(), "storage-link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Available(link); err == nil {
		t.Fatal("expected symlink storage path to fail closed")
	}
}

func TestSufficientUsesDocumentedMinimum(t *testing.T) {
	if Sufficient(MinimumAvailable-1) || !Sufficient(MinimumAvailable) {
		t.Fatal("unexpected capacity threshold")
	}
}
