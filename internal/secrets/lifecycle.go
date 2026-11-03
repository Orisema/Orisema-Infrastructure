// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

var lifecycleName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type LifecycleMetadata struct {
	Name      string
	ID        string
	Driver    string
	Managed   bool
	Transient bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PodmanManager struct {
	Binary string
}

func (p PodmanManager) Create(ctx context.Context, name string, input io.Reader) (LifecycleMetadata, error) {
	if !lifecycleName.MatchString(name) {
		return LifecycleMetadata{}, errors.New("secret name must be lowercase letters, digits or hyphens and start with a letter (max 63 chars)")
	}
	content, err := io.ReadAll(io.LimitReader(input, MaxSecretSize+1))
	if err != nil {
		return LifecycleMetadata{}, fmt.Errorf("read secret stdin: %w", err)
	}
	defer clear(content)
	if len(content) > MaxSecretSize {
		return LifecycleMetadata{}, fmt.Errorf("secret stdin exceeds %d-byte limit", MaxSecretSize)
	}
	bin := p.binary()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "secret", "create",
		"--label", "dev.orisema.managed=true",
		"--label", "dev.orisema.kind=secret",
		name, "-")
	cmd.Stdin = bytes.NewReader(content)
	out, err := cmd.Output()
	if err != nil {
		return LifecycleMetadata{}, fmt.Errorf("create Podman secret: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if !podmanSecretIdentity.MatchString(id) {
		_ = p.removeLocator(context.Background(), name)
		return LifecycleMetadata{}, errors.New("Podman returned invalid created secret identity")
	}
	item, err := p.Inspect(ctx, name)
	if err != nil || item.ID != id || !item.Managed || item.Transient {
		_ = p.Remove(context.Background(), id)
		if err != nil {
			return LifecycleMetadata{}, fmt.Errorf("verify created secret metadata: %w", err)
		}
		return LifecycleMetadata{}, errors.New("created secret metadata or ownership did not match")
	}
	return item, nil
}

func (p PodmanManager) List(ctx context.Context) ([]LifecycleMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, p.binary(), "secret", "ls", "--quiet").Output()
	if err != nil {
		return nil, fmt.Errorf("list Podman secret metadata: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) > 1024 {
		return nil, errors.New("Podman secret count exceeds 1024-item inspection limit")
	}
	items := make([]LifecycleMetadata, 0, len(fields))
	for _, id := range fields {
		if !podmanSecretIdentity.MatchString(id) {
			return nil, errors.New("Podman returned invalid secret identity while listing")
		}
		item, err := p.Inspect(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (p PodmanManager) Inspect(ctx context.Context, locator string) (LifecycleMetadata, error) {
	record, err := inspectPodmanSecret(ctx, p.Binary, locator)
	if err != nil {
		return LifecycleMetadata{}, err
	}
	return LifecycleMetadata{
		Name: record.Spec.Name, ID: record.ID, Driver: record.Spec.Driver.Name,
		Managed:   record.Spec.Labels["dev.orisema.managed"] == "true" && record.Spec.Labels["dev.orisema.kind"] == "secret",
		Transient: record.Spec.Labels["dev.orisema.transient"] == "true",
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func (p PodmanManager) Remove(ctx context.Context, identity string) error {
	if !podmanSecretIdentity.MatchString(identity) {
		return errors.New("invalid Podman secret identity")
	}
	return p.removeLocator(ctx, identity)
}

func (p PodmanManager) removeLocator(ctx context.Context, locator string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, p.binary(), "secret", "rm", locator).Run(); err != nil {
		return fmt.Errorf("remove Podman secret metadata locator %q: %w", locator, err)
	}
	return nil
}

func (p PodmanManager) binary() string {
	if p.Binary != "" {
		return p.Binary
	}
	return "podman"
}
