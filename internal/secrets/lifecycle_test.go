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

func lifecycleManager(t *testing.T, content string) PodmanManager {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_LIFECYCLE_HELPER", "1")
	t.Setenv("ORISEMA_LIFECYCLE_NAME", "managed-token")
	t.Setenv("ORISEMA_LIFECYCLE_CONTENT", content)
	return PodmanManager{Binary: binary}
}

func TestPodmanManagerLifecycleMetadataOnly(t *testing.T) {
	const canary = "lifecycle-canary-must-not-leak"
	manager := lifecycleManager(t, canary)
	created, err := manager.Create(context.Background(), "managed-token", strings.NewReader(canary))
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "managed-token" || created.ID != "abcdef0123456789abcdef012" || !created.Managed || created.Transient {
		t.Fatalf("created = %#v", created)
	}
	items, err := manager.List(context.Background())
	if err != nil || len(items) != 1 || items[0] != created {
		t.Fatalf("List() = %#v, %v", items, err)
	}
	if strings.Contains(created.Name+created.ID+created.Driver, canary) {
		t.Fatal("lifecycle metadata exposed content")
	}
	marker := filepath.Join(t.TempDir(), "removed")
	t.Setenv("ORISEMA_LIFECYCLE_REMOVE_MARKER", marker)
	if err := manager.Remove(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != created.ID {
		t.Fatalf("remove marker = %q, %v", got, err)
	}
}

func TestPodmanManagerRejectsOversizedInputBeforeMutation(t *testing.T) {
	content := strings.Repeat("x", MaxSecretSize+1)
	manager := lifecycleManager(t, content)
	_, err := manager.Create(context.Background(), "managed-token", strings.NewReader(content))
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestPodmanManagerRejectsInvalidName(t *testing.T) {
	manager := lifecycleManager(t, "secret")
	if _, err := manager.Create(context.Background(), "../token", strings.NewReader("secret")); err == nil {
		t.Fatal("invalid lifecycle name accepted")
	}
}
