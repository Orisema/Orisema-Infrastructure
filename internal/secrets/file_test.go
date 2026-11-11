// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func safeSecretDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(home, ".orisema-secret-test-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func writeSecretFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	// Apply the requested fixture mode independently of the process umask. Some
	// negative tests intentionally require a group/world-readable source.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFileInspectorReturnsMetadataIdentityOnly(t *testing.T) {
	path := writeSecretFile(t, safeSecretDir(t), "token", "test-secret", 0o600)
	got, err := (FileInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderFile, Locator: path})
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != ProviderFile || got.Locator != path || got.Identity == "" || strings.Contains(got.Identity, "test-secret") {
		t.Fatalf("metadata = %#v", got)
	}
}

func TestFileInspectorRejectsUnsafeSources(t *testing.T) {
	t.Run("permissive mode", func(t *testing.T) {
		path := writeSecretFile(t, safeSecretDir(t), "token", "secret", 0o644)
		_, err := (FileInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderFile, Locator: path})
		if err == nil || !strings.Contains(err.Error(), "0600") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		dir := safeSecretDir(t)
		target := writeSecretFile(t, dir, "target", "secret", 0o600)
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := (FileInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderFile, Locator: link}); err == nil {
			t.Fatal("symlink unexpectedly accepted")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		dir := safeSecretDir(t)
		path := writeSecretFile(t, dir, "token", "secret", 0o600)
		if err := os.Link(path, filepath.Join(dir, "other")); err != nil {
			t.Fatal(err)
		}
		if _, err := (FileInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderFile, Locator: path}); err == nil || !strings.Contains(err.Error(), "hard link") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		path := writeSecretFile(t, safeSecretDir(t), "token", strings.Repeat("x", MaxSecretSize+1), 0o600)
		if _, err := (FileInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderFile, Locator: path}); err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestPreparerStreamsValidatedDescriptorAndCleansUp(t *testing.T) {
	const content = "descriptor-canary"
	path := writeSecretFile(t, safeSecretDir(t), "token", content, 0o600)
	metadata, err := (FileInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderFile, Locator: path})
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "removed")
	t.Setenv("ORISEMA_FILE_STAGE_HELPER", "1")
	t.Setenv("ORISEMA_EXPECTED_FILE_SECRET", content)
	t.Setenv("ORISEMA_FILE_CLEANUP_MARKER", marker)
	mount := ResolvedMount{Name: "token", Provider: ProviderFile, Locator: path, Identity: metadata.Identity, Target: "token"}
	prepared, cleanup, err := (Preparer{PodmanBinary: binary}).PrepareService(context.Background(), "app", "web", []ResolvedMount{mount})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 1 || prepared[0].RuntimeIdentity != "abcdef0123456789abcdef012" || prepared[0].Identity != metadata.Identity {
		t.Fatalf("prepared = %#v", prepared)
	}
	if err := cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if removed, err := os.ReadFile(marker); err != nil || string(removed) != prepared[0].RuntimeIdentity {
		t.Fatalf("cleanup marker = %q, %v", removed, err)
	}
}

func TestPreparerRejectsChangedFileIdentity(t *testing.T) {
	path := writeSecretFile(t, safeSecretDir(t), "token", "before", 0o600)
	metadata, err := (FileInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderFile, Locator: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("different-size"), 0o600); err != nil {
		t.Fatal(err)
	}
	mount := ResolvedMount{Name: "token", Provider: ProviderFile, Locator: path, Identity: metadata.Identity, Target: "token"}
	_, _, err = (Preparer{}).PrepareService(context.Background(), "app", "web", []ResolvedMount{mount})
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("error = %v", err)
	}
}
