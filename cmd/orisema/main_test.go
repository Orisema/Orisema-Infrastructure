// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/operation"
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

func TestVersionDoesNotRequireManifestOrRuntime(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"Orisema Infrastructure", "Version: development", "Commit: unknown", "Built: unknown", "Go: go", "OS/Arch: "} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("version output %q does not contain %q", stdout.String(), expected)
		}
	}
}

func TestVersionRejectsExtraArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"version", "extra"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestOperationDiagnosisIsReadOnlyAndConservative(t *testing.T) {
	root := filepath.Join(t.TempDir(), "operations")
	store, err := operation.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	op := operation.Operation{ID: "update-1", Application: "demo", Services: []string{"web"}, Type: "UPDATE", CreatedAt: now, Previous: "nginx:old", RecoveryData: "old-container-id", Resources: []operation.Resource{{Kind: "container", Name: "orisema-demo-web", ID: "old-container-id", Ownership: operation.OwnershipManaged, Application: "demo", Service: "web"}, {Kind: "volume", Name: "customer-data", ID: "volume-id", Ownership: operation.OwnershipExternal}}}
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	for i, state := range []operation.State{operation.Preparing, operation.Applying, operation.RecoveryRequired} {
		if _, err := store.Transition(op.ID, state, "", "candidate failed", now.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := directorySnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"operation", "diagnose", "--journal", root}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"update-1", "RECOVERY_REQUIRED", "intervention required: true", "automatic resume safe: false", "rollback candidate: true", "ownership evidence complete: true (managed=1; external=1)", "No runtime inspection or changes made"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("stdout %q missing %q", stdout.String(), expected)
		}
	}
	after, err := directorySnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("diagnosis changed journal\nbefore=%s\nafter=%s", before, after)
	}
}

func TestOperationDiagnosisFailsClosedAndValidatesUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"operation", "diagnose"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("usage code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"operation", "diagnose", "--journal", filepath.Join(t.TempDir(), "missing")}, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "real directory") {
		t.Fatalf("missing code=%d stderr=%q", code, stderr.String())
	}
	root := filepath.Join(t.TempDir(), "operations")
	store, err := operation.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(operation.Operation{ID: "broken", Application: "demo", Services: []string{"web"}, Type: "UPDATE"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"operation", "diagnose", "--journal", root}, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "decode operation journal") {
		t.Fatalf("corrupt code=%d stderr=%q", code, stderr.String())
	}
}

func directorySnapshot(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var result strings.Builder
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&result, "%s:%d:%o:%d;", entry.Name(), info.Size(), info.Mode().Perm(), info.ModTime().UnixNano())
		if strings.HasSuffix(entry.Name(), ".json") {
			data, err := os.ReadFile(filepath.Join(root, entry.Name()))
			if err != nil {
				return "", err
			}
			result.Write(data)
		}
	}
	return result.String(), nil
}

func TestDocumentedExitCodeClasses(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.yaml")
	invalid := filepath.Join(dir, "invalid.yaml")
	validData := "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"
	invalidData := "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n    environment: []\n"
	if err := os.WriteFile(valid, []byte(validData), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invalid, []byte(invalidData), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "success", args: []string{"validate", valid}, want: 0},
		{name: "validation failure", args: []string{"validate", invalid}, want: 1},
		{name: "unsupported JSON output", args: []string{"--output", "json", "validate", valid}, want: 2},
		{name: "invalid usage", args: nil, want: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tc.args, strings.NewReader(""), &stdout, &stderr); got != tc.want {
				t.Fatalf("run(%q) = %d, want %d; stdout=%q stderr=%q", tc.args, got, tc.want, stdout.String(), stderr.String())
			}
		})
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
