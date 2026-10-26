// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

// Deploy creates a new container only. It never replaces, removes, or updates
// an existing container. Podman is responsible for enforcing port conflicts.
func (c Client) Deploy(ctx context.Context, app string, s manifest.Service) (string, error) {
	name := "orisema-" + app + "-" + s.Name
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	// Check all containers, including stopped ones, before attempting creation.
	containers, err := c.List(ctx)
	if err != nil {
		return "", err
	}
	if _, exists := Find(containers, name); exists {
		return "", fmt.Errorf("container %q already exists; refusing to replace it", name)
	}

	args := []string{"run", "--detach", "--name", name,
		"--label", "dev.orisema.managed=true",
		"--label", "dev.orisema.app=" + app,
		"--label", "dev.orisema.service=" + s.Name,
		"--security-opt", "no-new-privileges",
	}
	if h := s.Healthcheck; h != nil {
		// Explicit JSON prevents Podman from interpreting commands such as "none"
		// or JSON-looking shell text as a different healthcheck mode.
		command, err := json.Marshal([]string{"CMD-SHELL", h.Command})
		if err != nil {
			return "", fmt.Errorf("encode healthcheck command: %w", err)
		}
		args = append(args, "--health-cmd", string(command),
			"--health-interval", h.Interval,
			"--health-timeout", h.Timeout,
			"--health-retries", strconv.Itoa(h.Retries))
	}
	for _, p := range s.Ports {
		args = append(args, "--publish", strconv.Itoa(p.Host)+":"+strconv.Itoa(p.Container)+"/tcp")
	}
	args = append(args, s.Image)
	// No shell interpolation; arguments are passed directly to Podman.
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("podman run failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
