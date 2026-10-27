// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
)

func TestHistoryRoundTripAndManifestIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	for _, name := range []string{"alpha", "beta"} {
		err := AppendHistory(path, HistoryRecord{Manifest: name, Service: "web", Action: diagnostic.ActionStart, Timestamp: now, Succeeded: name == "alpha"})
		if err != nil {
			t.Fatal(err)
		}
	}
	attempts, err := LoadHistory(path, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Service != "web" || !attempts[0].Succeeded {
		t.Fatalf("unexpected attempts: %#v", attempts)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestHistoryRejectsCorruptionAndSymlink(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(bad, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHistory(bad, "demo"); err == nil {
		t.Fatal("expected corrupt history to fail")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(bad, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHistory(link, "demo"); err == nil {
		t.Fatal("expected symlink to fail")
	}
}

func TestHistoryMissingFailsClosed(t *testing.T) {
	if _, err := LoadHistory(filepath.Join(t.TempDir(), "missing"), "demo"); err == nil {
		t.Fatal("expected missing history to fail")
	}
}
