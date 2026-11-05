// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Environment describes the local Podman privilege boundary. Rootful and
// rootless stores are intentionally treated as different environments.
type Environment struct {
	Rootless bool
}

// StoragePath returns the absolute Podman graph root without listing or
// changing its contents.
func (c Client) StoragePath(ctx context.Context) (string, error) {
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "info", "--format", "{{.Store.GraphRoot}}").Output()
	if err != nil {
		return "", fmt.Errorf("read Podman storage metadata: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("read Podman storage metadata: graph root is not a clean absolute path")
	}
	return path, nil
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
