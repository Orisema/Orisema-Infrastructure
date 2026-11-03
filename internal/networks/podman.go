// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package networks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"time"
)

const maxPodmanNetworkMetadata = 64 * 1024

var podmanNetworkIdentity = regexp.MustCompile(`^[a-f0-9]{64}$`)
var podmanNetworkName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type PodmanInspector struct {
	Binary string
}

func (p PodmanInspector) InspectMetadata(ctx context.Context, ref Reference) (Metadata, error) {
	if ref.Provider != ProviderPodman || !podmanNetworkName.MatchString(ref.Locator) {
		return Metadata{}, errors.New("invalid Podman network reference")
	}
	item, err := inspectPodmanNetwork(ctx, p.Binary, ref.Locator)
	if err != nil {
		return Metadata{}, err
	}
	if item.Name != ref.Locator {
		return Metadata{}, errors.New("Podman returned mismatched network metadata")
	}
	if item.Driver != "bridge" {
		return Metadata{}, fmt.Errorf("Podman network driver %q is unsupported; expected bridge", item.Driver)
	}
	return Metadata{Provider: ProviderPodman, Locator: item.Name, Identity: item.ID, Driver: item.Driver}, nil
}

type podmanNetworkRecord struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Driver string `json:"driver"`
}

func inspectPodmanNetwork(ctx context.Context, binary, locator string) (podmanNetworkRecord, error) {
	bin := binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "network", "inspect", locator)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return podmanNetworkRecord{}, fmt.Errorf("prepare Podman network metadata inspection: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return podmanNetworkRecord{}, fmt.Errorf("Podman network metadata unavailable: %w", err)
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, maxPodmanNetworkMetadata+1))
	if readErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return podmanNetworkRecord{}, fmt.Errorf("read Podman network metadata: %w", readErr)
	}
	if len(out) > maxPodmanNetworkMetadata {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return podmanNetworkRecord{}, errors.New("Podman network metadata exceeds 64 KiB limit")
	}
	if err := cmd.Wait(); err != nil {
		return podmanNetworkRecord{}, fmt.Errorf("Podman network metadata unavailable: %w", err)
	}
	var items []podmanNetworkRecord
	if err := json.Unmarshal(out, &items); err != nil {
		return podmanNetworkRecord{}, fmt.Errorf("decode Podman network metadata: %w", err)
	}
	if len(items) != 1 || !podmanNetworkIdentity.MatchString(items[0].ID) || !podmanNetworkName.MatchString(items[0].Name) || items[0].Driver == "" {
		return podmanNetworkRecord{}, errors.New("Podman returned invalid network metadata")
	}
	return items[0], nil
}
