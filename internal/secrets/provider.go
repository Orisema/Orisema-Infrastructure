// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package secrets defines metadata-only contracts for external secret
// providers. It deliberately has no API that returns secret content.
package secrets

import (
	"context"
	"fmt"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

type Provider string

const (
	ProviderPodman Provider = "podman"
	ProviderFile   Provider = "file"
)

// Reference identifies externally managed material without containing it.
type Reference struct {
	Provider Provider
	Locator  string
}

// Metadata is safe for planning and diagnostics. Identity identifies the
// external object, not its content, and must never be a value-derived digest.
type Metadata struct {
	Provider Provider
	Locator  string
	Identity string
}

// Inspector resolves only provider metadata. Delivery of an already validated
// file descriptor or runtime reference will be a separate, mutation-scoped
// contract so values cannot accidentally enter plans, errors or journals.
type Inspector interface {
	InspectMetadata(context.Context, Reference) (Metadata, error)
}

// ResolvedMount binds a logical declaration to one container target using
// provider metadata only. Identity is never derived from secret content.
type ResolvedMount struct {
	Name            string
	Provider        Provider
	Locator         string
	Identity        string
	Target          string
	RuntimeIdentity string
}

// ResolveManifest resolves only declarations consumed by a service. It never
// reads secret content and never mutates a provider.
func ResolveManifest(ctx context.Context, m *manifest.Manifest, inspectors map[Provider]Inspector) (map[string][]ResolvedMount, error) {
	declarations := make(map[string]manifest.SecretDeclaration, len(m.Secrets))
	for _, declaration := range m.Secrets {
		declarations[declaration.Name] = declaration
	}
	metadata := make(map[string]Metadata)
	resolved := make(map[string][]ResolvedMount, len(m.Services))
	for _, service := range m.Services {
		for _, binding := range service.Secrets {
			declaration := declarations[binding.Name]
			provider, locator := sourceReference(declaration.Source)
			inspector := inspectors[provider]
			if inspector == nil {
				return nil, fmt.Errorf("service %q secret %q: provider %q is unavailable", service.Name, binding.Name, provider)
			}
			cacheKey := string(provider) + "\x00" + locator
			item, ok := metadata[cacheKey]
			if !ok {
				var err error
				item, err = inspector.InspectMetadata(ctx, Reference{Provider: provider, Locator: locator})
				if err != nil {
					return nil, fmt.Errorf("service %q secret %q: inspect %s reference %q: %w", service.Name, binding.Name, provider, locator, err)
				}
				if item.Provider != provider || item.Locator != locator || item.Identity == "" {
					return nil, fmt.Errorf("service %q secret %q: provider returned mismatched metadata", service.Name, binding.Name)
				}
				metadata[cacheKey] = item
			}
			resolved[service.Name] = append(resolved[service.Name], ResolvedMount{
				Name: binding.Name, Provider: provider, Locator: locator,
				Identity: item.Identity, Target: binding.Target,
			})
		}
	}
	return resolved, nil
}

func sourceReference(source manifest.SecretSource) (Provider, string) {
	if source.Podman != nil {
		return ProviderPodman, source.Podman.Name
	}
	if source.File != nil {
		return ProviderFile, source.File.Path
	}
	return "", ""
}

// SameMounts compares authorization-relevant metadata in declaration order.
func SameMounts(a, b []ResolvedMount) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		left, right := a[i], b[i]
		left.RuntimeIdentity, right.RuntimeIdentity = "", ""
		if left != right {
			return false
		}
	}
	return true
}
