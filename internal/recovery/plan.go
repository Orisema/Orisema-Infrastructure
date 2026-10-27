// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package recovery turns diagnostics into read-only, typed operator guidance.
// It deliberately contains no runtime mutation methods.
package recovery

import "github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"

type Step struct {
	Name                 string
	Service              string
	Action               diagnostic.Action
	Reason               string
	Preconditions        []string
	Risk                 string
	RequiresConfirmation bool
}

// Build derives deterministic guidance from diagnostic reports. It does not
// query the runtime and cannot execute any recommendation.
func Build(reports []diagnostic.Report) []Step {
	steps := make([]Step, 0, len(reports))
	for _, report := range reports {
		step := Step{Name: report.Name, Service: report.Service, Action: report.Action, Reason: report.Failure}
		switch report.Action {
		case diagnostic.ActionNone:
			step.Preconditions = []string{"continue observation"}
			step.Risk = "none identified by the current diagnostic"
		case diagnostic.ActionInvestigate:
			step.Preconditions = []string{"inspect logs and runtime evidence", "identify the root cause before any mutation"}
			step.Risk = "acting without a root cause can hide or repeat the failure"
		case diagnostic.ActionStart:
			step.Preconditions = []string{"verify Orisema ownership", "verify declared configuration still matches", "inspect the previous exit and logs"}
			step.Risk = "starting may repeat the process failure or restore external traffic"
			step.RequiresConfirmation = true
		case diagnostic.ActionRedeployReview:
			step.Preconditions = []string{"review the declarative plan", "resolve ownership or drift", "protect persistent data before replacement"}
			step.Risk = "redeployment may replace stateful or externally visible resources"
			step.RequiresConfirmation = true
		case diagnostic.ActionRetryRuntime:
			step.Preconditions = []string{"restore Podman access", "retry read-only inspection"}
			step.Risk = "container state is unknown; mutation is unsafe"
		default:
			step.Action = diagnostic.ActionInvestigate
			step.Preconditions = []string{"review the unknown recommendation"}
			step.Risk = "unknown recommendation; no mutation is safe"
		}
		steps = append(steps, step)
	}
	return steps
}
