// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
 "context"
 "os"
 "testing"
)

// Opt-in read-only integration test against an existing test container.
// Run: ORISEMA_TEST_CONTAINER=orisema-hello-orisema-web go test -v ./internal/runtime
func TestPodmanIntegrationReadOnly(t *testing.T) {
 name := os.Getenv("ORISEMA_TEST_CONTAINER")
 if name == "" { t.Skip("set ORISEMA_TEST_CONTAINER to an existing test container name") }
 c := Client{}
 containers, err := c.List(context.Background())
 if err != nil { t.Fatalf("list Podman containers: %v", err) }
 if _, ok := Find(containers, name); !ok { t.Fatalf("test container %q not found", name) }
 item, err := c.Inspect(context.Background(), name)
 if err != nil { t.Fatalf("inspect test container: %v", err) }
 if item.Config.Image == "" { t.Fatal("inspect returned empty configured image") }
 if item.State.Status == "" { t.Fatal("inspect returned empty state status") }
 t.Logf("container=%s status=%s image=%s", name, item.State.Status, item.Config.Image)
}
