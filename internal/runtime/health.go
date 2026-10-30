// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

// HealthEvidence returns the configured-check presence and a conservative raw
// status suitable for the operation health evaluator. Ambiguous or unsupported
// runtime output is reported as unknown.
func (item Inspection) HealthEvidence() (bool, string) {
	h := item.Config.Healthcheck
	if h == nil || (len(h.Test) == 1 && h.Test[0] == "NONE") || len(h.Test) < 2 || (h.Test[0] != "CMD" && h.Test[0] != "CMD-SHELL") {
		return false, "unknown"
	}
	podman, compatible := "", ""
	if item.State.Healthcheck != nil {
		podman = item.State.Healthcheck.Status
	}
	if item.State.Health != nil {
		compatible = item.State.Health.Status
	}
	if podman != "" && compatible != "" && podman != compatible {
		return true, "unknown"
	}
	status := podman
	if status == "" {
		status = compatible
	}
	switch status {
	case "starting", "healthy", "unhealthy":
		return true, status
	default:
		return true, "unknown"
	}
}

// HealthStatus reports the last runtime health observation separately from the
// container's lifecycle. A running container alone is never evidence of health.
func (item Inspection) HealthStatus() string {
	h := item.Config.Healthcheck
	if h == nil || (len(h.Test) == 1 && h.Test[0] == "NONE") {
		return "not configured"
	}
	if !item.State.Running || item.State.Status != "running" {
		return "unavailable (container not running)"
	}
	if len(h.Test) < 2 || (h.Test[0] != "CMD" && h.Test[0] != "CMD-SHELL") {
		return "unavailable"
	}
	var podman, compatible string
	if item.State.Healthcheck != nil {
		podman = item.State.Healthcheck.Status
	}
	if item.State.Health != nil {
		compatible = item.State.Health.Status
	}
	if podman != "" && compatible != "" && podman != compatible {
		return "unavailable (conflicting runtime health fields)"
	}
	status := podman
	if status == "" {
		status = compatible
	}
	switch status {
	case "starting", "healthy", "unhealthy":
		return status
	default:
		return "unavailable"
	}
}
