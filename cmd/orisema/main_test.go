// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
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

func TestRecoveryInitCreatesEmptyJournalWithoutRuntime(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	historyPath := filepath.Join(dir, "history.jsonl")
	manifestData := []byte("apiVersion: orisema.dev/v1alpha1\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"recover", "init", manifestPath, "--history", historyPath, "--operator", "alice"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	attempts, err := recovery.LoadHistory(historyPath, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 0 {
		t.Fatalf("attempts=%#v", attempts)
	}
	if !strings.Contains(stdout.String(), "No runtime changes made") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestRecoveryApplyCancellationDoesNotRequireOrConsumeHistory(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	manifestData := []byte("apiVersion: orisema.dev/v1alpha1\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n    recovery:\n      maxAttempts: 3\n      minInterval: 5m\n      cooldown: 30m\n")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"recover", "apply", manifestPath, "web", "--history", filepath.Join(dir, "missing"), "--token", strings.Repeat("a", 32)}, strings.NewReader("NO\n"), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "reservation was not consumed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRecoveryHistoryReportsUnresolvedExecutionReadOnly(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	historyPath := filepath.Join(dir, "history.jsonl")
	manifestData := []byte("apiVersion: orisema.dev/v1alpha1\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n    recovery:\n      maxAttempts: 3\n      minInterval: 5m\n      cooldown: 30m\n")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if err := recovery.InitHistory(historyPath, "demo", "alice", now); err != nil {
		t.Fatal(err)
	}
	report := diagnostic.Report{Name: "orisema-demo-web", Service: "web", Configuration: "matches declared fields", Execution: "exited", Failure: "container is not running", Action: diagnostic.ActionStart}
	policy := &manifest.Recovery{MaxAttempts: 3, MinInterval: "5m", Cooldown: "30m"}
	prepared, err := recovery.Prepare(recovery.PrepareRequest{HistoryPath: historyPath, Manifest: "demo", Report: report, Policy: policy, Operator: "alice", Reason: "reviewed", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := recovery.BeginExecution(recovery.BeginRequest{HistoryPath: historyPath, Manifest: "demo", Token: prepared.Token, Report: report, Policy: policy, Now: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"recover", "history", manifestPath, "--history", historyPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "Unresolved executions: 1") || !strings.Contains(stdout.String(), prepared.Token) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
