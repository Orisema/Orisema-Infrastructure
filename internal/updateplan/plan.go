// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package updateplan turns declarative inspection into conservative,
// read-only update guidance. It has no runtime mutation capability.
package updateplan

import (
	"fmt"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/planner"
)

type Action string

const (
	Noop      Action = "NOOP"
	Candidate Action = "UPDATE_CANDIDATE"
	Blocked   Action = "BLOCKED"
)

type Step struct {
	Service      string
	Name         string
	CurrentImage string
	TargetImage  string
	Action       Action
	Reason       string
}

// Build classifies supplied planner evidence without querying a runtime. Only
// an image-only drift with a declared healthcheck is an update candidate.
func Build(m *manifest.Manifest, inspected []planner.Step) []Step {
	result := make([]Step, 0, len(m.Services))
	for i, service := range m.Services {
		name := "orisema-" + m.Name + "-" + service.Name
		step := Step{Service: service.Name, Name: name, TargetImage: service.Image, Action: Blocked}
		if i >= len(inspected) || inspected[i].Name != name {
			step.Reason = "planner evidence is missing or out of order"
			result = append(result, step)
			continue
		}
		evidence := inspected[i]
		step.CurrentImage = evidence.CurrentImage
		switch evidence.Action {
		case planner.Noop:
			step.Action = Noop
			step.Reason = "declared managed fields already match"
		case planner.Drift:
			if len(evidence.Differences) != 1 || evidence.Differences[0] != "configured image differs" {
				step.Reason = "update requires image-only drift; observed: " + evidence.Reason
			} else if evidence.CurrentImage == "" || evidence.CurrentImage == service.Image {
				step.Reason = "current image evidence is missing or inconsistent"
			} else if service.Healthcheck == nil {
				step.Reason = "a declared healthcheck is required before update can be considered"
			} else {
				step.Action = Candidate
				step.Reason = "ownership and supported configuration match; only the configured image differs"
			}
		case planner.Create:
			step.Reason = "container is missing; use deploy rather than update"
		case planner.Review:
			step.Reason = "runtime identity or ownership could not be safely established: " + evidence.Reason
		default:
			step.Reason = fmt.Sprintf("unsupported planner action %q", evidence.Action)
		}
		result = append(result, step)
	}
	return result
}
