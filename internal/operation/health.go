// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
	"fmt"
	"time"
)

type HealthStatus string

const (
	HealthPending            HealthStatus = "PENDING"
	HealthHealthy            HealthStatus = "HEALTHY"
	HealthUnhealthy          HealthStatus = "UNHEALTHY"
	HealthNotRunning         HealthStatus = "NOT_RUNNING"
	HealthNoCheck            HealthStatus = "NO_HEALTHCHECK"
	HealthRuntimeUnavailable HealthStatus = "RUNTIME_UNAVAILABLE"
	HealthUnknown            HealthStatus = "UNKNOWN"
	HealthTimedOut           HealthStatus = "TIMED_OUT"
)

type HealthPolicy struct {
	Timeout                 time.Duration
	MaxConsecutiveUnhealthy int
}

type HealthObservation struct {
	At                 time.Time
	RuntimeAvailable   bool
	Running            bool
	HealthcheckPresent bool
	Status             string
}

type HealthEvaluation struct {
	Status               HealthStatus
	Succeeded            bool
	Terminal             bool
	InterventionRequired bool
	ConsecutiveUnhealthy int
	Deadline             time.Time
	Reason               string
}

// EvaluateHealth deterministically evaluates supplied observations. It never
// polls, sleeps, retries, queries a runtime or mutates an operation.
func EvaluateHealth(startedAt, now time.Time, policy HealthPolicy, observations []HealthObservation) (HealthEvaluation, error) {
	if startedAt.IsZero() || now.IsZero() || now.Before(startedAt) {
		return HealthEvaluation{}, errors.New("valid verification start and current timestamps are required")
	}
	if policy.Timeout < time.Second || policy.Timeout > 30*time.Minute {
		return HealthEvaluation{}, errors.New("health verification timeout must be between 1 second and 30 minutes")
	}
	if policy.MaxConsecutiveUnhealthy < 1 || policy.MaxConsecutiveUnhealthy > 10 {
		return HealthEvaluation{}, errors.New("maximum consecutive unhealthy observations must be between 1 and 10")
	}
	startedAt, now = startedAt.UTC(), now.UTC()
	deadline := startedAt.Add(policy.Timeout)
	result := HealthEvaluation{Status: HealthPending, Deadline: deadline, Reason: "waiting for a conclusive configured healthcheck observation"}
	previous := startedAt
	considered := false
	for i, observation := range observations {
		if observation.At.IsZero() || observation.At.Before(startedAt) || observation.At.Before(previous) || observation.At.After(now) {
			return HealthEvaluation{}, fmt.Errorf("health observation %d has an invalid or out-of-order timestamp", i+1)
		}
		previous = observation.At
		if observation.At.After(deadline) {
			continue
		}
		considered = true
		if !observation.RuntimeAvailable {
			result.Status = HealthRuntimeUnavailable
			result.InterventionRequired = true
			result.ConsecutiveUnhealthy = 0
			result.Reason = "runtime was unavailable; health cannot be inferred"
			continue
		}
		if !observation.Running {
			result.Status, result.Terminal, result.InterventionRequired = HealthNotRunning, true, true
			result.Reason = "container is not running"
			return result, nil
		}
		if !observation.HealthcheckPresent {
			result.Status, result.Terminal, result.InterventionRequired = HealthNoCheck, true, true
			result.Reason = "no configured healthcheck is available; running is not proof of health"
			return result, nil
		}
		switch observation.Status {
		case "healthy":
			result.Status, result.Succeeded, result.Terminal = HealthHealthy, true, true
			result.InterventionRequired = false
			result.ConsecutiveUnhealthy = 0
			result.Reason = "configured healthcheck reported healthy within the verification window"
			return result, nil
		case "unhealthy":
			result.Status = HealthUnhealthy
			result.InterventionRequired = false
			result.ConsecutiveUnhealthy++
			result.Reason = "configured healthcheck reported unhealthy"
			if result.ConsecutiveUnhealthy >= policy.MaxConsecutiveUnhealthy {
				result.Terminal, result.InterventionRequired = true, true
				result.Reason = "consecutive unhealthy observation limit reached"
				return result, nil
			}
		case "starting":
			result.Status = HealthPending
			result.InterventionRequired = false
			result.ConsecutiveUnhealthy = 0
			result.Reason = "configured healthcheck is still starting"
		case "", "unknown":
			result.Status = HealthUnknown
			result.InterventionRequired = true
			result.ConsecutiveUnhealthy = 0
			result.Reason = "configured healthcheck result is unknown"
		default:
			return HealthEvaluation{}, fmt.Errorf("health observation %d has unsupported status %q", i+1, observation.Status)
		}
	}
	if !now.Before(deadline) {
		result.Status, result.Terminal, result.InterventionRequired = HealthTimedOut, true, true
		if !considered {
			result.Reason = "verification timed out without an observation"
		} else {
			result.Reason = "verification timed out without a healthy observation"
		}
	}
	return result, nil
}
