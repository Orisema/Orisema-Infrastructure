// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwned(t *testing.T) {
	var item Inspection
	item.Config.Labels = map[string]string{"dev.orisema.managed": "true", "dev.orisema.app": "demo", "dev.orisema.service": "web"}
	if !Owned(item, "demo", "web") {
		t.Fatal("expected matching ownership")
	}
	if Owned(item, "demo", "db") {
		t.Fatal("unexpected service ownership")
	}
	if Owned(item, "other", "web") {
		t.Fatal("unexpected application ownership")
	}
	delete(item.Config.Labels, "dev.orisema.managed")
	if Owned(item, "demo", "web") {
		t.Fatal("missing managed label accepted")
	}
}

func TestControlExactMutatesByAdmittedID(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "called")
	bin := filepath.Join(dir, "podman")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = inspect ]; then
  printf '[{"Id":"container-exact","Config":{"Labels":{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web"}}}]\n'
  exit 0
fi
printf '%%s %%s\n' "$1" "$2" > %q
`, marker)
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	client := Client{Binary: bin}
	if err := client.ControlExact(context.Background(), "demo", "web", "different", "start"); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("identity mismatch error=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("identity mismatch mutated runtime: %v", err)
	}
	if err := client.ControlExact(context.Background(), "demo", "web", "container-exact", "start"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "start container-exact\n" {
		t.Fatalf("runtime target=%q err=%v", data, err)
	}
}
