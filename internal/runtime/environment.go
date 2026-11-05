// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Environment describes the local Podman privilege boundary. Rootful and
// rootless stores are intentionally treated as different environments.
type Environment struct {
	Rootless bool
}

// Environment reads only Podman host metadata using a fixed template.
func (c Client) Environment(ctx context.Context) (Environment, error) {
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "info", "--format", "{{.Host.Security.Rootless}}").Output()
	if err != nil {
		return Environment{}, fmt.Errorf("read Podman environment: %w", err)
	}
	switch strings.TrimSpace(string(out)) {
	case "true":
		return Environment{Rootless: true}, nil
	case "false":
		return Environment{Rootless: false}, nil
	default:
		return Environment{}, errors.New("read Podman environment: unrecognized rootless mode")
	}
}
