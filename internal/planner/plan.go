// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package planner builds a conservative, read-only plan from Podman state.
package planner

import (
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

type Action string

const (
	Create Action = "CREATE"
	Review Action = "REVIEW"
)

type Step struct {
	Name             string
	Image            string
	CurrentImage     string
	CurrentID        string
	State            string
	Action           Action
	Differences      []string
	TypedDifferences []Difference
	Reason           string
}

func Build(m *manifest.Manifest, existing []runtime.Container) []Step {
	steps := make([]Step, 0, len(m.Services))
	for _, s := range m.Services {
		name := "orisema-" + m.Name + "-" + s.Name
		if len(s.PublicEnvironment) != 0 || len(s.Networks) != 0 {
			steps = append(steps, Step{Name: name, Image: s.Image, Action: Review, Reason: "public configuration or network delivery is declared but not implemented; no changes made"})
			continue
		}
		if len(s.Secrets) != 0 {
			steps = append(steps, Step{Name: name, Image: s.Image, Action: Review, Reason: "secret delivery is declared but not implemented; no changes made"})
			continue
		}
		found, exists := runtime.Find(existing, name)
		if !exists {
			steps = append(steps, Step{Name: name, Image: s.Image, Action: Create, Reason: "container not found"})
			continue
		}
		// An existing name is not evidence of matching image digest, port mappings,
		// labels or ownership. Never classify it as safe to update or replace.
		steps = append(steps, Step{Name: name, Image: s.Image, State: found.State, Action: Review,
			Reason: "container name exists; configuration and ownership not verified"})
	}
	return steps
}
