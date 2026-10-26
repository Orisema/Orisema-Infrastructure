// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package diagnostic evaluates observed service conditions without changing
// containers. Declarative planning remains the responsibility of planner.
package diagnostic

import (
	"context"
	"fmt"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/planner"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

const repeatedRestartThreshold = 3

type Runtime interface {
	List(context.Context) ([]runtime.Container, error)
	Inspect(context.Context, string) (runtime.Inspection, error)
}

// Action is a typed operational recommendation. It never authorizes or
// performs the corresponding operation.
type Action string

const (
	ActionNone           Action = "NONE"
	ActionInvestigate    Action = "INVESTIGATE"
	ActionStart          Action = "START"
	ActionRedeployReview Action = "REDEPLOY_REVIEW"
	ActionRetryRuntime   Action = "RETRY_RUNTIME"
)

// Report keeps desired configuration, observed execution, application health,
// detected failure and recommended operator action as independent dimensions.
type Report struct {
	Name           string
	Configuration  string
	Execution      string
	Health         string
	Failure        string
	Recommendation string
	Action         Action
}

// Build performs a read-only diagnostic. A list failure produces an explicit
// report for every declared service because absence cannot be inferred safely.
func Build(ctx context.Context, m *manifest.Manifest, client Runtime) ([]Report, error) {
	containers, err := client.List(ctx)
	if err != nil {
		reports := make([]Report, 0, len(m.Services))
		for _, svc := range m.Services {
			reports = append(reports, Report{
				Name:           containerName(m.Name, svc.Name),
				Configuration:  "unknown",
				Execution:      "unavailable",
				Health:         "unavailable",
				Failure:        fmt.Sprintf("runtime query failed: %v", err),
				Recommendation: "verify Podman access and retry diagnosis",
				Action:         ActionRetryRuntime,
			})
		}
		return reports, err
	}

	reports := make([]Report, 0, len(m.Services))
	for _, svc := range m.Services {
		name := containerName(m.Name, svc.Name)
		found, exists := runtime.Find(containers, name)
		if !exists {
			reports = append(reports, Report{
				Name:           name,
				Configuration:  "absent",
				Execution:      "not found",
				Health:         "unavailable",
				Failure:        "container does not exist",
				Recommendation: "review the plan and deploy with explicit approval",
				Action:         ActionRedeployReview,
			})
			continue
		}

		item, inspectErr := client.Inspect(ctx, name)
		if inspectErr != nil {
			reports = append(reports, Report{
				Name:           name,
				Configuration:  "review",
				Execution:      observedListState(found.State),
				Health:         "unavailable",
				Failure:        fmt.Sprintf("container inspection failed: %v", inspectErr),
				Recommendation: "inspect the container manually; do not modify it automatically",
				Action:         ActionRetryRuntime,
			})
			continue
		}

		step := planner.BuildInspected(ctx, &manifest.Manifest{Name: m.Name, Services: []manifest.Service{svc}}, []runtime.Container{found}, fixedInspector{item: item})[0]
		report := Report{
			Name:          name,
			Configuration: configurationStatus(step),
			Execution:     executionStatus(item),
			Health:        item.HealthStatus(),
			Failure:       "none detected",
		}
		report.Failure, report.Recommendation, report.Action = assess(step, item, report.Health)
		reports = append(reports, report)
	}
	return reports, nil
}

type fixedInspector struct{ item runtime.Inspection }

func (i fixedInspector) Inspect(context.Context, string) (runtime.Inspection, error) {
	return i.item, nil
}

func containerName(app, service string) string { return "orisema-" + app + "-" + service }

func configurationStatus(step planner.Step) string {
	switch step.Action {
	case planner.Noop:
		return "matches declared fields"
	case planner.Drift:
		return "drift: " + step.Reason
	default:
		return "review: " + step.Reason
	}
}

func observedListState(state string) string {
	if state == "" {
		return "unknown"
	}
	return state + " (from container list)"
}

func executionStatus(item runtime.Inspection) string {
	if item.State.Status == "" {
		return "unknown"
	}
	return item.State.Status
}

func assess(step planner.Step, item runtime.Inspection, health string) (string, string, Action) {
	if step.Action == planner.Review {
		return "ownership or configuration could not be verified", "review ownership and configuration manually", ActionInvestigate
	}
	if step.Action == planner.Drift {
		return "declared configuration drift", "review drift; Orisema will not replace the container automatically", ActionRedeployReview
	}
	if !item.State.Running || item.State.Status != "running" {
		return "container is not running", "investigate the exit, then use start only with explicit approval", ActionStart
	}
	if item.RestartCount >= repeatedRestartThreshold {
		return fmt.Sprintf("repeated restarts observed (%d total)", item.RestartCount), "investigate logs and restart history; no automatic restart is recommended", ActionInvestigate
	}
	switch health {
	case "healthy":
		return "none detected", "none", ActionNone
	case "unhealthy":
		return "application healthcheck is unhealthy", "investigate healthcheck output and application logs", ActionInvestigate
	case "not configured":
		return "healthcheck is not configured", "consider declaring an application healthcheck", ActionInvestigate
	case "starting":
		return "healthcheck has not completed", "wait for the healthcheck and run diagnosis again", ActionNone
	default:
		return "application health is unavailable", "inspect runtime health data manually", ActionInvestigate
	}
}
