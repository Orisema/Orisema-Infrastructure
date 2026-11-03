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
	item, err := inspectPodmanSecret(ctx, p.Binary, ref.Locator)
	if err != nil {
		return Metadata{}, err
	}
	if item.Spec.Name != ref.Locator {
		return Metadata{}, errors.New("Podman returned mismatched secret metadata")
	}
	return Metadata{Provider: ProviderPodman, Locator: item.Spec.Name, Identity: item.ID}, nil
}

type podmanSecretRecord struct {
	ID        string    `json:"ID"`
	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
	Spec      struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
		Driver struct {
			Name string `json:"Name"`
		} `json:"Driver"`
	} `json:"Spec"`
}

func inspectPodmanSecret(ctx context.Context, binary, locator string) (podmanSecretRecord, error) {
	bin := binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "secret", "inspect", locator).Output()
	if err != nil {
		return podmanSecretRecord{}, fmt.Errorf("Podman secret metadata unavailable: %w", err)
	}
	var items []podmanSecretRecord
	if err := json.Unmarshal(out, &items); err != nil {
		return podmanSecretRecord{}, fmt.Errorf("decode Podman secret metadata: %w", err)
	}
	if len(items) != 1 || !podmanSecretIdentity.MatchString(items[0].ID) || items[0].Spec.Name == "" {
		return podmanSecretRecord{}, errors.New("Podman returned invalid secret metadata")
	}
	return items[0], nil
}
