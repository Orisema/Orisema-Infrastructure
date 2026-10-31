// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAuditIsPrivateDurableAndContainsNoOperationalOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "remote.jsonl")
	record := AuditRecord{RequestID: "request", Phase: "RESULT", Host: "primary", Action: "logs", Application: "demo", Service: "web", Outcome: OutcomeOK, DurationMS: 12}
	if err := AppendAudit(path, record); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"version":"orisema.remote.audit/v1"`, `"phase":"RESULT"`, `"outcome":"OK"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("audit missing %s: %s", want, text)
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

func TestAppendAuditRejectsUnsafeTargets(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := AppendAudit(filepath.Join(dir, "audit"), AuditRecord{}); err == nil {
		t.Fatal("writable parent accepted")
	}
	safe := t.TempDir()
	if err := os.Chmod(safe, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(safe, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(safe, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := AppendAudit(link, AuditRecord{}); err == nil {
		t.Fatal("symlink accepted")
	}
}
