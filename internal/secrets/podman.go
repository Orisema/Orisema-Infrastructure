// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"time"
)

var podmanSecretIdentity = regexp.MustCompile(`^[a-f0-9]{25,64}$`)

// PodmanInspector reads the default metadata-only secret representation. It
// deliberately exposes no flag or method corresponding to --showsecret.
type PodmanInspector struct {
	Binary string
}

func (p PodmanInspector) InspectMetadata(ctx context.Context, ref Reference) (Metadata, error) {
	if ref.Provider != ProviderPodman || ref.Locator == "" {
		return Metadata{}, errors.New("invalid Podman secret reference")
	}
	bin := p.Binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "secret", "inspect", ref.Locator).Output()
	if err != nil {
		return Metadata{}, fmt.Errorf("Podman secret metadata unavailable: %w", err)
	}
	var items []struct {
		ID   string `json:"ID"`
		Spec struct {
			Name string `json:"Name"`
		} `json:"Spec"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return Metadata{}, fmt.Errorf("decode Podman secret metadata: %w", err)
	}
	if len(items) != 1 || !podmanSecretIdentity.MatchString(items[0].ID) || items[0].Spec.Name != ref.Locator {
		return Metadata{}, errors.New("Podman returned mismatched secret metadata")
	}
	return Metadata{Provider: ProviderPodman, Locator: items[0].Spec.Name, Identity: items[0].ID}, nil
}
