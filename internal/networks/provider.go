// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package networks resolves metadata for external networks. It exposes no
// lifecycle operation: external networks are never created, changed or removed.
package networks

import (
	"context"
	"fmt"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

type Provider string

const ProviderPodman Provider = "podman"

type Reference struct {
	Provider Provider
	Locator  string
}

// Metadata contains only the fields needed to prove that a supported external
// network still has the same identity. It deliberately excludes IPAM details.
type Metadata struct {
	Provider Provider
	Locator  string
	Identity string
	Driver   string
}

type Inspector interface {
	InspectMetadata(context.Context, Reference) (Metadata, error)
}

type ResolvedBinding struct {
	Name     string
	Provider Provider
	Locator  string
	Identity string
	Driver   string
}

// ResolveManifest resolves only declarations consumed by services and caches
// repeated external references. It never mutates Podman.
func ResolveManifest(ctx context.Context, m *manifest.Manifest, inspectors map[Provider]Inspector) (map[string][]ResolvedBinding, error) {
	declarations := make(map[string]manifest.NetworkDeclaration, len(m.Networks))
	for _, declaration := range m.Networks {
		declarations[declaration.Name] = declaration
	}
	metadata := make(map[string]Metadata)
	resolved := make(map[string][]ResolvedBinding, len(m.Services))
	for _, service := range m.Services {
		for _, binding := range service.Networks {
			declaration, ok := declarations[binding.Name]
			if !ok {
				return nil, fmt.Errorf("service %q network %q is not declared", service.Name, binding.Name)
			}
			provider, locator := sourceReference(declaration.Source)
			inspector := inspectors[provider]
			if inspector == nil {
				return nil, fmt.Errorf("service %q network %q: provider %q is unavailable", service.Name, binding.Name, provider)
			}
			cacheKey := string(provider) + "\x00" + locator
			item, cached := metadata[cacheKey]
			if !cached {
				var err error
				item, err = inspector.InspectMetadata(ctx, Reference{Provider: provider, Locator: locator})
				if err != nil {
					return nil, fmt.Errorf("service %q network %q: inspect %s reference %q: %w", service.Name, binding.Name, provider, locator, err)
				}
				if item.Provider != provider || item.Locator != locator || item.Identity == "" || item.Driver != "bridge" {
					return nil, fmt.Errorf("service %q network %q: provider returned mismatched metadata", service.Name, binding.Name)
				}
				metadata[cacheKey] = item
			}
			resolved[service.Name] = append(resolved[service.Name], ResolvedBinding{
				Name: binding.Name, Provider: provider, Locator: locator,
				Identity: item.Identity, Driver: item.Driver,
			})
		}
	}
	return resolved, nil
}

func sourceReference(source manifest.NetworkSource) (Provider, string) {
	if source.Podman != nil {
		return ProviderPodman, source.Podman.Name
	}
	return "", ""
}

// SameBindings compares authorization-relevant network metadata in declaration
// order. A changed locator, identity or driver invalidates prior evidence.
func SameBindings(a, b []ResolvedBinding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RevalidateManifest resolves fresh metadata and rejects any identity change.
// Callers must invoke it immediately before a future mutation.
func RevalidateManifest(ctx context.Context, m *manifest.Manifest, inspectors map[Provider]Inspector, expected map[string][]ResolvedBinding) error {
	fresh, err := ResolveManifest(ctx, m, inspectors)
	if err != nil {
		return err
	}
	for _, service := range m.Services {
		if !SameBindings(expected[service.Name], fresh[service.Name]) {
			return fmt.Errorf("service %q network identity changed; no changes made", service.Name)
		}
	}
	return nil
}
