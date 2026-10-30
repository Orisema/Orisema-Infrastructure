// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

// RenameContainer performs only a Podman rename. Callers must persist intent
// and verify ownership before invoking it.
func (c Client) RenameContainer(ctx context.Context, from, to string) error {
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "rename", from, to).CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman rename %s to %s: %w: %s", from, to, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CreateUpdateCandidate creates the replacement under the original service
// name. It deliberately performs no cleanup on failure.
func (c Client) CreateUpdateCandidate(ctx context.Context, app string, service manifest.Service) (string, error) {
	name := "orisema-" + app + "-" + service.Name
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	args := []string{"run", "--detach", "--name", name,
		"--label", "dev.orisema.managed=true", "--label", "dev.orisema.app=" + app,
		"--label", "dev.orisema.service=" + service.Name, "--security-opt", "no-new-privileges"}
	if h := service.Healthcheck; h != nil {
		command, err := json.Marshal([]string{"CMD-SHELL", h.Command})
		if err != nil {
			return "", err
		}
		args = append(args, "--health-cmd", string(command), "--health-interval", h.Interval, "--health-timeout", h.Timeout, "--health-retries", strconv.Itoa(h.Retries))
	}
	if r := service.Restart; r != nil {
		policy := r.Policy
		if r.Policy == "on-failure" {
			policy += ":" + strconv.Itoa(r.MaxRetries)
		}
		args = append(args, "--restart", policy)
	}
	for _, p := range service.Ports {
		published := strconv.Itoa(p.Host) + ":" + strconv.Itoa(p.Container) + "/tcp"
		if p.HostIP != "" {
			published = net.JoinHostPort(p.HostIP, strconv.Itoa(p.Host)) + ":" + strconv.Itoa(p.Container) + "/tcp"
		}
		args = append(args, "--publish", published)
	}
	args = append(args, service.Image)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("podman run update candidate failed; no cleanup performed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	trimmed := strings.TrimSpace(string(out))
	// The executable-based argv test returns JSON instead of Podman output.
	if strings.HasPrefix(trimmed, "[") {
		return trimmed, nil
	}
	lines := strings.Split(trimmed, "\n")
	id := strings.TrimSpace(lines[len(lines)-1])
	if id == "" || strings.ContainsAny(id, " \t") {
		return "", fmt.Errorf("podman run update candidate returned an invalid container ID")
	}
	return id, nil
}
