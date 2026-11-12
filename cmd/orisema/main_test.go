// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/operation"
	"github.com/Orisema/Orisema-Infrastructure/internal/publicconfig"
	"github.com/Orisema/Orisema-Infrastructure/internal/recovery"
)

func TestHelpAndOperationalInventoryCoverEveryPublicCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, arg := range []string{"help", "--help", "-h"} {
		stdout.Reset()
		stderr.Reset()
		if code := run([]string{arg}, strings.NewReader(""), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("%s code=%d stderr=%q", arg, code, stderr.String())
		}
		for _, command := range publicCommandPaths {
			if !strings.Contains(stdout.String(), "  "+command+"\n") {
				t.Fatalf("help missing public command %q", command)
			}
		}
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	inventoryPath := filepath.Join(filepath.Dir(source), "..", "..", "docs", "CLI-INVENTORY.md")
	inventory, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range publicCommandPaths {
		marker := "<!-- command: " + command + " -->"
		if strings.Count(string(inventory), marker) != 1 {
			t.Fatalf("inventory marker %q must occur exactly once", marker)
		}
	}
}

func TestMaintainerAndAgentContractDocumentsStayConnected(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	required := map[string][]string{
		"AGENTS.md": {
			"Não invente resultados", "Preserve o comportamento fail-closed",
			"Uma revisão humana é obrigatória", "docs/MAINTAINER-MANUAL.md",
		},
		filepath.Join(".github", "pull_request_template.md"): {
			"## Problema e escopo", "## Módulos e contratos afetados",
			"## Riscos, efeitos e recuperação", "## Evidência de validação",
			"## Contrato de segurança", "## Revisão humana",
		},
		filepath.Join("docs", "MAINTAINER-MANUAL.md"): {
			"## Invariantes", "## Fluxo de um comando", "## Mapa dos subsistemas",
			"## Validação proporcional ao risco", "## Incidente e recuperação",
		},
		filepath.Join("docs", "OPERATOR-AGENT-LABS.md"): {
			"## Pré-requisitos comuns", "## Laboratório 1", "## Laboratório 2",
			"## Laboratório 3", "## Registro de evidência",
		},
	}
	for name, markers := range required {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, marker := range markers {
			if !bytes.Contains(data, []byte(marker)) {
				t.Errorf("%s is missing contract marker %q", name, marker)
			}
		}
	}
}

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

func TestOperationInitRequiresAbsolutePathAndDoesNotRepair(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"operation", "init", "--journal", "relative"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("relative path code=%d stderr=%q", code, stderr.String())
	}

	root := filepath.Join(t.TempDir(), "operations")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"operation", "init", "--journal", root}, strings.NewReader(""), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "No manifest loaded and no runtime changes made") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if _, err := operation.OpenExistingWritableStore(root); err != nil {
		t.Fatalf("initialized journal is unusable: %v", err)
	}

	lockPath := filepath.Join(root, ".store.lock")
	if err := os.Chmod(lockPath, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"operation", "init", "--journal", root}, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "unsafe") {
		t.Fatalf("unsafe state code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	info, err := os.Stat(lockPath)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("init repaired unsafe lock: %v, %v", info, err)
	}
}

func TestJournaledStartStopCompletesOnlyAfterVerification(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "operations")
	if _, err := operation.InitializeStore(journal); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "state")
	if err := os.WriteFile(statePath, []byte("stopped\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	podman := filepath.Join(dir, "podman")
	script := fmt.Sprintf(`#!/bin/sh
state=$(cat %q)
case "$1" in
  inspect)
    running=false
    status=exited
    if [ "$state" = running ]; then running=true; status=running; fi
    printf '[{"Id":"container-1","Name":"orisema-demo-web","State":{"Status":"%%s","Running":%%s},"Config":{"Image":"nginx:stable","Labels":{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web"}},"HostConfig":{"PortBindings":{},"RestartPolicy":{}},"NetworkSettings":{"Networks":{}}}]\n' "$status" "$running"
    ;;
  start) printf 'running\n' > %q ;;
  stop) printf 'stopped\n' > %q ;;
  *) exit 1 ;;
esac
`, statePath, statePath, statePath)
	if err := os.WriteFile(podman, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr bytes.Buffer
	code := run([]string{"start", "--journal", journal, manifestPath, "web"}, strings.NewReader("START\n"), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "completed and verified") {
		t.Fatalf("start code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"stop", "--journal", journal, manifestPath, "web"}, strings.NewReader("STOP\n"), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "completed and verified") {
		t.Fatalf("stop code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	store, err := operation.OpenExistingStore(journal)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := store.List()
	if err != nil || len(operations) != 2 {
		t.Fatalf("operations=%#v err=%v", operations, err)
	}
	for _, op := range operations {
		if op.State != operation.Completed || op.Control == nil || len(op.Events) != 5 {
			t.Fatalf("incomplete control evidence: %#v", op)
		}
	}
}

func TestJournaledControlCancellationAndMissingStoreDoNotTouchRuntime(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "mutated")
	podman := filepath.Join(dir, "podman")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = inspect ]; then printf '[{\"Id\":\"container-1\",\"State\":{\"Running\":false},\"Config\":{\"Labels\":{\"dev.orisema.managed\":\"true\",\"dev.orisema.app\":\"demo\",\"dev.orisema.service\":\"web\"}}}]\\n'; else touch %q; fi\n", marker)
	if err := os.WriteFile(podman, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(dir, "missing")
	if code := run([]string{"start", "--journal", missing, manifestPath, "web"}, strings.NewReader("START\n"), &stdout, &stderr); code != 1 {
		t.Fatalf("missing store code=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing store was created: %v", err)
	}
	journal := filepath.Join(dir, "operations")
	if _, err := operation.InitializeStore(journal); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"start", "--journal", journal, manifestPath, "web"}, strings.NewReader("NO\n"), &stdout, &stderr); code != 1 {
		t.Fatalf("cancel code=%d stderr=%q", code, stderr.String())
	}
	store, err := operation.OpenExistingStore(journal)
	if err != nil {
		t.Fatal(err)
	}
	if operations, err := store.List(); err != nil || len(operations) != 0 {
		t.Fatalf("cancellation admitted operation: %#v, %v", operations, err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime mutation occurred: %v", err)
	}
}

func doctorPodmanScript(storagePath string, rootless bool) string {
	return fmt.Sprintf("#!/bin/sh\ncase \"$1:$3\" in\n  *Rootless*) printf '%t\\n' ;;\n  *GraphRoot*) printf '%%s\\n' %q ;;\n  ps:*) printf '[]\\n' ;;\n  *) exit 1 ;;\nesac\n", rootless, storagePath)
}

func TestDoctorReportsPassWithoutChangingJournal(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "operations")
	if _, err := operation.OpenStore(journal); err != nil {
		t.Fatal(err)
	}
	before, err := directorySnapshot(journal)
	if err != nil {
		t.Fatal(err)
	}
	podman := filepath.Join(dir, "podman")
	if err := os.WriteFile(podman, []byte(doctorPodmanScript(dir, true)), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", manifestPath, "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, expected := range []string{"orisema-doctor-v2", "[PASS] manifest.valid code=OK", "mode=rootless", "[PASS] operation.pending code=OK", "[PASS] binding.secrets code=OK", "[PASS] binding.networks code=OK", "code=CONTAINER_ABSENT", "Doctor result: WARN (exit=0)", "No runtime or journal changes made"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("stdout %q missing %q", stdout.String(), expected)
		}
	}
	after, err := directorySnapshot(journal)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("doctor changed journal\nbefore=%s\nafter=%s", before, after)
	}
}

func TestDoctorBlocksIncompleteApplicationOperation(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "operations")
	store, err := operation.OpenStore(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(operation.Operation{ID: "update-pending", Application: "demo", Services: []string{"web"}, Type: "UPDATE"}); err != nil {
		t.Fatal(err)
	}
	podman := filepath.Join(dir, "podman")
	if err := os.WriteFile(podman, []byte(doctorPodmanScript(dir, false)), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", manifestPath, "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "[BLOCK] operation.pending code=OPERATION_INCOMPLETE: id=update-pending type=UPDATE state=PENDING") || !strings.Contains(stdout.String(), "mode=rootful") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestDoctorDoesNotExposeSecretLocator(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	canary := "doctor-secret-locator-canary"
	manifestData := "apiVersion: orisema.dev/v1alpha3\nkind: Application\nname: demo\nsecrets:\n  - name: token\n    source:\n      file:\n        path: /private/" + canary + "\nservices:\n  - name: web\n    image: nginx:stable\n    secrets:\n      - name: token\n        target: token\n"
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "operations")
	if _, err := operation.OpenStore(journal); err != nil {
		t.Fatal(err)
	}
	podman := filepath.Join(dir, "podman")
	if err := os.WriteFile(podman, []byte(doctorPodmanScript(dir, true)), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", manifestPath, "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stdout.String(), "code=SECRET_BINDING_UNAVAILABLE") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) {
		t.Fatalf("doctor exposed secret locator: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestDoctorFailsClosedForUnsafeJournalAndUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"doctor"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("usage code=%d stderr=%q", code, stderr.String())
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	podman := filepath.Join(dir, "podman")
	if err := os.WriteFile(podman, []byte("#!/bin/sh\nprintf 'true\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	stdout.Reset()
	stderr.Reset()
	code := run([]string{"doctor", manifestPath, "--journal", filepath.Join(dir, "missing")}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stdout.String(), "code=JOURNAL_UNAVAILABLE") || !strings.Contains(stdout.String(), "do not recreate or delete evidence") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestDoctorClassifiesJournalFailuresWithoutRepair(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	podman := filepath.Join(dir, "podman")
	if err := os.WriteFile(podman, []byte(doctorPodmanScript(dir, true)), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	unsafeJournal := filepath.Join(dir, "unsafe")
	if _, err := operation.OpenStore(unsafeJournal); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafeJournal, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"doctor", manifestPath, "--journal", unsafeJournal}, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stdout.String(), "code=JOURNAL_UNSAFE") {
		t.Fatalf("unsafe code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if info, err := os.Stat(unsafeJournal); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("doctor repaired unsafe journal: %v, %v", info, err)
	}

	corruptJournal := filepath.Join(dir, "corrupt")
	if _, err := operation.OpenStore(corruptJournal); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptJournal, "broken.json"), []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"doctor", manifestPath, "--journal", corruptJournal}, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stdout.String(), "code=JOURNAL_CORRUPT") {
		t.Fatalf("corrupt code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestDoctorRedactsInvalidManifestDetails(t *testing.T) {
	dir := t.TempDir()
	canary := "manifest-secret-canary-value"
	manifestPath := filepath.Join(dir, "manifest.yaml")
	data := "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nunknownSensitiveField: " + canary + "\nservices:\n  - name: web\n    image: nginx:stable\n"
	if err := os.WriteFile(manifestPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "operations")
	if _, err := operation.OpenStore(journal); err != nil {
		t.Fatal(err)
	}
	podman := filepath.Join(dir, "podman")
	if err := os.WriteFile(podman, []byte(doctorPodmanScript(dir, true)), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", manifestPath, "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stdout.String(), "code=MANIFEST_INVALID") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) {
		t.Fatalf("doctor exposed invalid manifest content: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestUpdatePlanReportsImageOnlyCandidateWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	manifestData := "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:new\n    healthcheck:\n      command: nginx -t\n      interval: 30s\n      timeout: 5s\n      retries: 3\n"
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls")
	updateState := filepath.Join(dir, "candidate-created")
	podmanPath := filepath.Join(dir, "podman")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
case "$1" in
  ps) printf '%%s\n' '[{"Names":["orisema-demo-web"],"State":"running","Image":"nginx:old"}]' ;;
  inspect)
    if [ -f %q ] && [ "$4" = "orisema-demo-web" ]; then
      printf '%%s\n' '[{"Id":"candidate-456","Name":"orisema-demo-web","State":{"Status":"running","Running":true,"Healthcheck":{"Status":"healthy"}},"Config":{"Image":"nginx:new","Labels":{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web"},"Healthcheck":{"Test":["CMD-SHELL","nginx -t"],"Interval":30000000000,"Timeout":5000000000,"Retries":3}},"HostConfig":{"PortBindings":{},"RestartPolicy":{"Name":"no","MaximumRetryCount":0}}}]'
    elif [ -f %q ]; then
      printf '%%s\n' "[{\"Id\":\"container-123\",\"Name\":\"$4\",\"State\":{\"Status\":\"exited\",\"Running\":false},\"Config\":{\"Image\":\"nginx:old\",\"Labels\":{\"dev.orisema.managed\":\"true\",\"dev.orisema.app\":\"demo\",\"dev.orisema.service\":\"web\"},\"Healthcheck\":{\"Test\":[\"CMD-SHELL\",\"nginx -t\"],\"Interval\":30000000000,\"Timeout\":5000000000,\"Retries\":3}},\"HostConfig\":{\"PortBindings\":{},\"RestartPolicy\":{\"Name\":\"no\",\"MaximumRetryCount\":0}}}]"
    else
      printf '%%s\n' '[{"Id":"container-123","Name":"orisema-demo-web","State":{"Status":"running","Running":true},"Config":{"Image":"nginx:old","Labels":{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web"},"Healthcheck":{"Test":["CMD-SHELL","nginx -t"],"Interval":30000000000,"Timeout":5000000000,"Retries":3}},"HostConfig":{"PortBindings":{},"RestartPolicy":{"Name":"no","MaximumRetryCount":0}}}]'
    fi ;;
  stop|rename) ;;
  run) touch %q; printf '%%s\n' 'candidate-456' ;;
  *) exit 91 ;;
esac
`, logPath, updateState, updateState, updateState)
	if err := os.WriteFile(podmanPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	code := run([]string{"update", "plan", manifestPath}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "[UPDATE_CANDIDATE]") || !strings.Contains(stdout.String(), "nginx:old -> nginx:new") || !strings.Contains(stdout.String(), "differences: IMAGE fingerprint=") || !strings.Contains(stdout.String(), "No journal created") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "ps ") || !strings.HasPrefix(lines[1], "inspect ") {
		t.Fatalf("unexpected Podman calls: %q", calls)
	}
	stdout.Reset()
	journal := filepath.Join(dir, "operations")
	code = run([]string{"update", "prepare", manifestPath, "web", "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "Update intent persisted") || !strings.Contains(stdout.String(), "No confirmation requested") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	store, err := operation.OpenExistingStore(journal)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := store.Incomplete()
	if err != nil || len(operations) != 1 || operations[0].Intent == nil || operations[0].Intent.Service != "web" || operations[0].Resources[0].ID != "container-123" {
		t.Fatalf("operations=%#v err=%v", operations, err)
	}
	beforeCancel, err := directorySnapshot(journal)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "authorize", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader("NO\n"), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "journal and runtime unchanged") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	afterCancel, _ := directorySnapshot(journal)
	if beforeCancel != afterCancel {
		t.Fatal("cancelled authorization changed journal")
	}
	stdout.Reset()
	stderr.Reset()
	confirmation := "AUTHORIZE UPDATE demo/web\n"
	code = run([]string{"update", "authorize", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader(confirmation), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "state=PREPARING") || !strings.Contains(stdout.String(), "No image pulled or runtime changes made") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	authorized, err := store.Load(operations[0].ID)
	if err != nil || authorized.State != operation.Preparing || authorized.Authorization == nil {
		t.Fatalf("authorized=%#v err=%v", authorized, err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "apply", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader("APPLY UPDATE demo/web\n"), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "candidate-456") || !strings.Contains(stdout.String(), "Previous container preserved") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	applied, err := store.Load(operations[0].ID)
	if err != nil || applied.State != operation.Applying || applied.Execution == nil || applied.Execution.CandidateID != "candidate-456" || !applied.Execution.PreviousRenamed {
		t.Fatalf("applied=%#v err=%v", applied, err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "verify", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "decision: COMPLETE") || !strings.Contains(stdout.String(), "state=COMPLETED") || !strings.Contains(stdout.String(), "health=HEALTHY") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	verified, err := store.Load(operations[0].ID)
	if err != nil || verified.State != operation.Completed || verified.Decision == nil || len(verified.Health.VerificationResources) != 2 {
		t.Fatalf("verified=%#v err=%v", verified, err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"operation", "diagnose", "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "none") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestConfiguredUpdatePreparePersistsV2AndAuthorizeRejectsChangedIdentity(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	manifestData := "apiVersion: orisema.dev/v1alpha4\nkind: Application\nname: demo\nsecrets:\n  - name: token\n    source:\n      podman:\n        name: demo-token\nnetworks:\n  - name: private\n    source:\n      podman:\n        name: demo-private\nservices:\n  - name: web\n    image: nginx:new\n    publicEnvironment:\n      - name: APP_MODE\n        value: laboratory\n    secrets:\n      - name: token\n        target: token\n    networks:\n      - name: private\n    healthcheck:\n      command: nginx -t\n      interval: 30s\n      timeout: 5s\n      retries: 3\n"
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	secretID := "abcdef0123456789abcdef012"
	networkID := strings.Repeat("b", 64)
	_, environmentDigest := publicconfig.Metadata([]manifest.PublicEnvironmentEntry{{Name: "APP_MODE", Value: "laboratory"}})
	identityChanged := filepath.Join(dir, "identity-changed")
	candidateCreated := filepath.Join(dir, "candidate-created")
	podmanPath := filepath.Join(dir, "podman")
	script := fmt.Sprintf(`#!/bin/sh
case "$1 $2" in
  "secret inspect")
    if [ -f %q ]; then secret_id=abcdef0123456789abcdef013; else secret_id=%s; fi
    printf '[{"ID":"%%s","Spec":{"Name":"demo-token"}}]\n' "$secret_id" ;;
  "network inspect") printf '[{"name":"demo-private","id":"%s","driver":"bridge"}]\n' ;;
  "ps -a")
    if [ -f %q ]; then image=nginx:new; else image=nginx:old; fi
    printf '[{"Names":["orisema-demo-web"],"State":"running","Image":"%%s"}]\n' "$image" ;;
  "inspect --type")
    if [ -f %q ] && [ "$4" = "orisema-demo-web" ]; then
      id=candidate-configured-456 image=nginx:new status=running running=true health=',"Healthcheck":{"Status":"healthy"}'
    elif [ -f %q ]; then
      id=container-123 image=nginx:old status=exited running=false health=
    else
      id=container-123 image=nginx:old status=running running=true health=
    fi
    printf '[{"Id":"%%s","Name":"%%s","State":{"Status":"%%s","Running":%%s%%s},"Config":{"Image":"%%s","Env":["APP_MODE=laboratory"],"Labels":{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web","dev.orisema.public-env.names":"APP_MODE","dev.orisema.public-env.hash":"%s","dev.orisema.secret.token":"podman:%s","dev.orisema.network.private":"podman:%s:bridge"},"Healthcheck":{"Test":["CMD-SHELL","nginx -t"],"Interval":30000000000,"Timeout":5000000000,"Retries":3}},"HostConfig":{"PortBindings":{},"RestartPolicy":{"Name":"no","MaximumRetryCount":0}},"NetworkSettings":{"Networks":{"demo-private":{}}}}]\n' "$id" "$4" "$status" "$running" "$health" "$image" ;;
  "stop orisema-demo-web"|"rename orisema-demo-web") ;;
  "run --detach") touch %q; printf 'candidate-configured-456\n' ;;
  *) exit 91 ;;
esac
`, identityChanged, secretID, networkID, candidateCreated, candidateCreated, candidateCreated, environmentDigest, secretID, networkID, candidateCreated)
	if err := os.WriteFile(podmanPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	journal := filepath.Join(dir, "operations")
	var stdout, stderr bytes.Buffer
	code := run([]string{"update", "prepare", manifestPath, "web", "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	store, err := operation.OpenExistingStore(journal)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := store.Incomplete()
	if err != nil || len(operations) != 1 || operations[0].Intent == nil || operations[0].Intent.Contract != "orisema-update-intent-v2" || operations[0].Intent.Bindings == nil {
		t.Fatalf("operations=%#v err=%v", operations, err)
	}
	journalData, err := os.ReadFile(filepath.Join(journal, operations[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"laboratory", "demo-token", "demo-private"} {
		if strings.Contains(string(journalData), forbidden) {
			t.Fatalf("journal exposed value or locator %q: %s", forbidden, journalData)
		}
	}
	before := string(journalData)
	if err := os.WriteFile(identityChanged, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "authorize", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader("AUTHORIZE UPDATE demo/web\n"), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "update authorization blocked") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(filepath.Join(journal, operations[0].ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if before != string(after) {
		t.Fatal("failed configured authorization changed journal")
	}
	if err := os.Remove(identityChanged); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "authorize", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader("AUTHORIZE UPDATE demo/web\n"), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "state=PREPARING") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	authorized, err := store.Load(operations[0].ID)
	if err != nil || authorized.State != operation.Preparing || authorized.Authorization == nil || authorized.Authorization.PlanFingerprint != authorized.Intent.PlanFingerprint {
		t.Fatalf("authorized=%#v err=%v", authorized, err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "apply", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader("APPLY UPDATE demo/web\n"), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "candidate-configured-456") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	applied, err := store.Load(operations[0].ID)
	if err != nil || applied.Execution == nil || !applied.Execution.BindingsPrepared || applied.Execution.CandidateID != "candidate-configured-456" {
		t.Fatalf("applied=%#v err=%v", applied, err)
	}
	if err := os.WriteFile(identityChanged, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeVerify, err := directorySnapshot(journal)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "verify", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "configured binding identity changed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	afterVerify, _ := directorySnapshot(journal)
	if beforeVerify != afterVerify {
		t.Fatal("failed configured verification changed journal")
	}
	if err := os.Remove(identityChanged); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "verify", manifestPath, "web", "--journal", journal, "--operation", operations[0].ID}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "decision: COMPLETE") || !strings.Contains(stdout.String(), "state=COMPLETED") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestConfiguredUpdateVerifyRequiresIntactJournalWithoutExposingValues(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
	manifestData := "apiVersion: orisema.dev/v1alpha4\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:new\n    publicEnvironment:\n      - name: APP_MODE\n        value: laboratory\n"
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"update", "verify", manifestPath, "web", "--journal", filepath.Join(t.TempDir(), "operations"), "--operation", "update-test"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "update continuation blocked before confirmation") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "laboratory") {
		t.Fatalf("update error exposed public value: %q", stderr.String())
	}
}

func TestPublicConfigurationPreparePersistsReviewOnlyIntentWithoutValue(t *testing.T) {
	dir := t.TempDir()
	const canary = "public-config-canary-value"
	manifestPath := filepath.Join(dir, "manifest.yaml")
	manifestData := "apiVersion: orisema.dev/v1alpha4\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n    publicEnvironment:\n      - name: APP_MODE\n        value: " + canary + "\n    healthcheck:\n      command: nginx -t\n      interval: 30s\n      timeout: 5s\n      retries: 3\n"
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	names, digest := publicconfig.Metadata([]manifest.PublicEnvironmentEntry{{Name: "APP_MODE", Value: canary}})
	podman := filepath.Join(dir, "podman")
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  ps) printf '%%s\n' '[{"Names":["orisema-demo-web"],"State":"running","Image":"nginx:stable"}]' ;;
  inspect) printf '%%s\n' '[{"Id":"container-123","Name":"orisema-demo-web","State":{"Status":"running","Running":true},"Config":{"Image":"nginx:stable","Env":["APP_MODE=old"],"Labels":{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web","%s":"%s","%s":"%s"},"Healthcheck":{"Test":["CMD-SHELL","nginx -t"],"Interval":30000000000,"Timeout":5000000000,"Retries":3}},"HostConfig":{"PortBindings":{},"RestartPolicy":{"Name":"no","MaximumRetryCount":0}},"NetworkSettings":{"Networks":{}}}]' ;;
  *) exit 91 ;;
esac
`, publicconfig.NamesLabel, names, publicconfig.HashLabel, digest)
	if err := os.WriteFile(podman, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	journal := filepath.Join(dir, "operations")
	if _, err := operation.OpenStore(journal); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"update", "prepare", manifestPath, "web", "--journal", journal}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "Review-only public configuration intent persisted") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(journal, strings.TrimSpace(strings.Split(strings.Split(stdout.String(), "Update intent persisted: ")[1], "\n")[0])+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), canary) || strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) {
		t.Fatalf("public configuration value leaked: journal=%s stdout=%q stderr=%q", data, stdout.String(), stderr.String())
	}
	var persisted operation.Operation
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"update", "authorize", manifestPath, "web", "--journal", journal, "--operation", persisted.ID}, strings.NewReader("AUTHORIZE UPDATE demo/web\n"), &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "blocked before confirmation") || !strings.Contains(stderr.String(), "review-only") {
		t.Fatalf("authorize code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
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

func TestOperationRecoveryFailsClosedForUnsupportedState(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	manifestData := "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:stable\n"
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "operations")
	store, err := operation.OpenStore(journal)
	if err != nil {
		t.Fatal(err)
	}
	op := operation.Operation{ID: "update-unsupported", Application: "demo", Services: []string{"web"}, Type: "UPDATE", CreatedAt: time.Now().UTC()}
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"recover", "operation", manifestPath, "--journal", journal, "--operation", op.ID}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "not a supported RECOVERY_REQUIRED update") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"recover", "operation"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("usage code=%d stderr=%q", code, stderr.String())
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
