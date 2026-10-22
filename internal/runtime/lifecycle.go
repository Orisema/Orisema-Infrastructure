// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
 "context"
 "fmt"
 "os/exec"
 "time"
)

// Owned verifies that an existing container belongs to this exact application
// and service. A matching name alone is not sufficient authorization.
func Owned(item Inspection, app, service string) bool {
 labels := item.Config.Labels
 return labels["dev.orisema.managed"] == "true" &&
  labels["dev.orisema.app"] == app &&
  labels["dev.orisema.service"] == service
}

// Control changes the state of an existing, verified Orisema container.
// Only start and stop are accepted. Never creates or removes containers.
func (c Client) Control(ctx context.Context, app, service, operation string) error {
 if operation != "start" && operation != "stop" {
  return fmt.Errorf("unsupported operation %q", operation)
 }
 name := "orisema-" + app + "-" + service
 item, err := c.Inspect(ctx, name)
 if err != nil { return err }
 if !Owned(item, app, service) { return fmt.Errorf("%s is not verified as an Orisema-owned container", name) }
 bin := c.Binary
 if bin == "" { bin = "podman" }
 ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
 defer cancel()
 out, err := exec.CommandContext(ctx, bin, operation, name).CombinedOutput()
 if err != nil { return fmt.Errorf("podman %s %s: %w: %s", operation, name, err, string(out)) }
 return nil
}

// Logs reads a bounded tail of an existing Orisema-owned container's logs.
func (c Client) Logs(ctx context.Context, app, service string) ([]byte, error) {
 name := "orisema-" + app + "-" + service
 item, err := c.Inspect(ctx, name)
 if err != nil { return nil, err }
 if !Owned(item, app, service) { return nil, fmt.Errorf("%s is not verified as an Orisema-owned container", name) }
 bin := c.Binary
 if bin == "" { bin = "podman" }
 ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
 defer cancel()
 out, err := exec.CommandContext(ctx, bin, "logs", "--tail", "100", name).CombinedOutput()
 if err != nil { return nil, fmt.Errorf("podman logs %s: %w", name, err) }
 return out, nil
}
