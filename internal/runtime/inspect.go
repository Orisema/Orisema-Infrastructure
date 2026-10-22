// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
 "context"
 "encoding/json"
 "fmt"
 "os/exec"
 "time"
)

type Binding struct {
 HostIP string `json:"HostIp"`
 HostPort string `json:"HostPort"`
}
type Inspection struct {
 Name string `json:"Name"`
 Config struct {
  Image string `json:"Image"`
  Labels map[string]string `json:"Labels"`
 } `json:"Config"`
 HostConfig struct {
  PortBindings map[string][]Binding `json:"PortBindings"`
 } `json:"HostConfig"`
}

// Inspect reads a container's configured image reference, ownership labels,
// and host port bindings. No mutation is performed.
func (c Client) Inspect(ctx context.Context, name string) (Inspection, error) {
 bin := c.Binary
 if bin == "" { bin = "podman" }
 ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
 defer cancel()
 out, err := exec.CommandContext(ctx, bin, "inspect", "--type", "container", name).Output()
 if err != nil { return Inspection{}, fmt.Errorf("inspect container %q: %w", name, err) }
 var items []Inspection
 if err := json.Unmarshal(out, &items); err != nil { return Inspection{}, fmt.Errorf("decode inspect output: %w", err) }
 if len(items) != 1 { return Inspection{}, fmt.Errorf("expected one inspected container, got %d", len(items)) }
 return items[0], nil
}
