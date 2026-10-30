// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/operation"
)

const testManifest = "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: backup-lab\nservices:\n  - name: web\n    image: docker.io/library/nginx:1.27-alpine\n"

func backupFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(testManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, manifestPath, filepath.Join(root, "artifact")
}

func TestCreateVerifyAndRestoreConfiguration(t *testing.T) {
	root, manifestPath, artifact := backupFixture(t)
	meta, err := Create(CreateRequest{ManifestPath: manifestPath, ArtifactPath: artifact, JournalPath: filepath.Join(root, "journal")})
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Metadata.BackupID != meta.BackupID || string(verified.Payload["manifest.yaml"]) != testManifest {
		t.Fatalf("verified=%#v", verified.Metadata)
	}
	destination := filepath.Join(root, "restored")
	restored, err := Restore(RestoreRequest{ArtifactPath: artifact, DestinationPath: destination, JournalPath: filepath.Join(root, "restore-journal")})
	if err != nil {
		t.Fatal(err)
	}
	if restored.BackupID != meta.BackupID {
		t.Fatal("restored wrong backup")
	}
	data, err := os.ReadFile(filepath.Join(destination, "manifest.yaml"))
	if err != nil || string(data) != testManifest {
		t.Fatalf("data=%q err=%v", data, err)
	}
	for _, path := range []string{artifact, filepath.Join(artifact, "payload"), destination} {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode=%o", path, info.Mode().Perm())
		}
	}
}

func TestCorruptionAndUnsafeEntriesAreRejectedBeforeRestore(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, artifact string)
	}{
		{"payload", func(t *testing.T, a string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(a, "payload", "manifest.yaml"), []byte("corrupt"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unexpected", func(t *testing.T, a string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(a, "extra"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsafe-mode", func(t *testing.T, a string) {
			t.Helper()
			if err := os.Chmod(filepath.Join(a, "metadata.json"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink", func(t *testing.T, a string) {
			t.Helper()
			p := filepath.Join(a, "payload", "manifest.yaml")
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../metadata.json", p); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown-metadata", func(t *testing.T, a string) {
			t.Helper()
			p := filepath.Join(a, "metadata.json")
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			data = []byte(strings.Replace(string(data), "{", "{\n  \"unknown\": true,", 1))
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if err := os.WriteFile(filepath.Join(a, "metadata.sha256"), []byte(hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"duplicate-metadata-key", func(t *testing.T, a string) {
			t.Helper()
			p := filepath.Join(a, "metadata.json")
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			data = []byte(strings.Replace(string(data), "{", "{\n  \"format\": \"orisema.backup/v1alpha1\",", 1))
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if err := os.WriteFile(filepath.Join(a, "metadata.sha256"), []byte(hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"fifo-payload", func(t *testing.T, a string) {
			t.Helper()
			p := filepath.Join(a, "payload", "manifest.yaml")
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(p, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, manifestPath, artifact := backupFixture(t)
			if _, err := Create(CreateRequest{ManifestPath: manifestPath, ArtifactPath: artifact, JournalPath: filepath.Join(root, "journal")}); err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, artifact)
			if _, err := Verify(artifact); err == nil {
				t.Fatal("unsafe artifact verified")
			}
			destination := filepath.Join(root, "restored")
			if _, err := Restore(RestoreRequest{ArtifactPath: artifact, DestinationPath: destination, JournalPath: filepath.Join(root, "restore-journal")}); err == nil {
				t.Fatal("unsafe artifact restored")
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatalf("destination exists: %v", err)
			}
		})
	}
}

func TestRestoreNeverOverwritesExistingDestination(t *testing.T) {
	root, manifestPath, artifact := backupFixture(t)
	if _, err := Create(CreateRequest{ManifestPath: manifestPath, ArtifactPath: artifact, JournalPath: filepath.Join(root, "journal")}); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "existing")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(destination, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(RestoreRequest{ArtifactPath: artifact, DestinationPath: destination, JournalPath: filepath.Join(root, "restore-journal")}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v", err)
	}
	data, _ := os.ReadFile(sentinel)
	if string(data) != "keep" {
		t.Fatal("sentinel changed")
	}
	if _, err := os.Lstat(filepath.Join(root, "restore-journal")); !os.IsNotExist(err) {
		t.Fatal("rejected restore created journal")
	}
}

func TestRenameNoReplacePreservesRacingDestination(t *testing.T) {
	root := t.TempDir()
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	if err := os.Mkdir(from, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(to, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := renameNoReplace(from, to); err == nil {
		t.Fatal("rename replaced destination")
	}
	if _, err := os.Stat(from); err != nil {
		t.Fatal("source disappeared")
	}
	if _, err := os.Stat(to); err != nil {
		t.Fatal("destination disappeared")
	}
}

func TestOptionalOperationHistoryIsValidatedAndRestoredInert(t *testing.T) {
	root, manifestPath, artifact := backupFixture(t)
	source := filepath.Join(root, "source-journal")
	store, err := operation.OpenStore(source)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	op, err := store.CreateFileOperation(operation.FileOperationRequest{ID: "backup-history-source", Type: "BACKUP", Application: "backup-lab", BackupID: "backup-source", Source: "/manifest", SourceSHA256: strings.Repeat("a", 64), Artifact: "/historical", Staging: "/historical.staging-backup-history-source", FirstStep: "stage", At: now})
	if err != nil {
		t.Fatal(err)
	}
	steps := [][2]string{{"stage", "payload"}, {"payload", "metadata"}, {"metadata", "publish"}, {"publish", "verify"}, {"verify", ""}}
	for i, step := range steps {
		if _, err := store.RecordFileOperationStep(op.ID, step[0], step[1], now.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := Create(CreateRequest{ManifestPath: manifestPath, ArtifactPath: artifact, JournalPath: filepath.Join(root, "journal"), OperationsPath: source})
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Scope.OperationHistory {
		t.Fatal("history scope absent")
	}
	verified, err := Verify(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified.Payload["operations.jsonl"]) == 0 {
		t.Fatal("history payload empty")
	}
	destination := filepath.Join(root, "restored")
	if _, err := Restore(RestoreRequest{ArtifactPath: artifact, DestinationPath: destination, JournalPath: filepath.Join(root, "restore-journal")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "operations.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, ".store.lock")); !os.IsNotExist(err) {
		t.Fatal("restored history became an active operation store")
	}
}
