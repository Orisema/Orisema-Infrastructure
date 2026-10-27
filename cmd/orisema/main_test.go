// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/recovery"
)

func TestRecoveryRecordWritesScopedHistoryWithoutRuntime(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	historyPath := filepath.Join(dir, "history.jsonl")
	manifestData := []byte("apiVersion: orisema.dev/v1alpha1\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n    recovery:\n      maxAttempts: 3\n      minInterval: 5m\n      cooldown: 30m\n")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"recover", "record", manifestPath, "web", "START", "failed", "--history", historyPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	attempts, err := recovery.LoadHistory(historyPath, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Service != "web" || attempts[0].Succeeded {
		t.Fatalf("unexpected attempts: %#v", attempts)
	}
	if !strings.Contains(stdout.String(), "No runtime changes made") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
