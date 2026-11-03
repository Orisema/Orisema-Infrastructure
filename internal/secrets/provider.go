// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package secrets defines metadata-only contracts for external secret
// providers. It deliberately has no API that returns secret content.
package secrets

import "context"

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
