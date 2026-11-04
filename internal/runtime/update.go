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
	"github.com/Orisema/Orisema-Infrastructure/internal/networks"
	"github.com/Orisema/Orisema-Infrastructure/internal/publicconfig"
	"github.com/Orisema/Orisema-Infrastructure/internal/secrets"
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
	return c.CreateUpdateCandidateWithResolved(ctx, app, service, nil, nil)
}

// CreateUpdateCandidateWithResolved creates a replacement using the complete
// resolved configuration. It deliberately performs no container cleanup on
// failure so update recovery evidence remains available.
func (c Client) CreateUpdateCandidateWithResolved(ctx context.Context, app string, service manifest.Service, mounts []secrets.ResolvedMount, networkBindings []networks.ResolvedBinding) (string, error) {
	if len(service.Secrets) != len(mounts) || len(service.Networks) != len(networkBindings) {
		return "", fmt.Errorf("resolved update binding metadata is incomplete; no changes made")
	}
	name := "orisema-" + app + "-" + service.Name
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	args := []string{"run", "--detach", "--name", name,
		"--label", "dev.orisema.managed=true", "--label", "dev.orisema.app=" + app,
		"--label", "dev.orisema.service=" + service.Name, "--security-opt", "no-new-privileges"}
	if len(service.PublicEnvironment) != 0 {
		names, digest := publicconfig.Metadata(service.PublicEnvironment)
		args = append(args, "--label", publicconfig.NamesLabel+"="+names, "--label", publicconfig.HashLabel+"="+digest)
		for _, entry := range service.PublicEnvironment {
			args = append(args, "--env", entry.Name+"="+entry.Value)
		}
	}
	for i, binding := range networkBindings {
		if binding.Name != service.Networks[i].Name || binding.Provider != networks.ProviderPodman || binding.Identity == "" || binding.Driver != "bridge" {
			return "", fmt.Errorf("network %q has unsupported or incomplete resolved metadata; no changes made", service.Networks[i].Name)
		}
		args = append(args, "--label", networkLabel(binding.Name)+"="+string(binding.Provider)+":"+binding.Identity+":"+binding.Driver, "--network", binding.Identity)
	}
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
	for _, mount := range mounts {
		runtimeIdentity := mount.RuntimeIdentity
		if runtimeIdentity == "" {
			runtimeIdentity = mount.Identity
		}
		if (mount.Provider != secrets.ProviderPodman && mount.Provider != secrets.ProviderFile) || mount.Identity == "" || runtimeIdentity == "" {
			return "", fmt.Errorf("secret %q has unsupported or incomplete resolved metadata", mount.Name)
		}
		args = append(args, "--label", secretLabel(mount.Target)+"="+string(mount.Provider)+":"+mount.Identity, "--secret", runtimeIdentity+",target="+mount.Target+",type=mount")
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
