// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"strings"
	"testing"
	"time"
)

func healthFixture() (time.Time, HealthPolicy) {
	return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 2}
}

func TestEvaluateHealthOnlyConfiguredHealthySucceeds(t *testing.T) {
	start, policy := healthFixture()
	result, err := EvaluateHealth(start, start.Add(10*time.Second), policy, []HealthObservation{{At: start.Add(5 * time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "healthy"}})
	if err != nil || !result.Succeeded || !result.Terminal || result.Status != HealthHealthy {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestEvaluateHealthFailsConservatively(t *testing.T) {
	start, policy := healthFixture()
	tests := []struct {
		name         string
		observations []HealthObservation
		want         HealthStatus
		terminal     bool
	}{
		{"not running", []HealthObservation{{At: start, RuntimeAvailable: true}}, HealthNotRunning, true},
		{"no healthcheck", []HealthObservation{{At: start, RuntimeAvailable: true, Running: true}}, HealthNoCheck, true},
		{"runtime unavailable", []HealthObservation{{At: start, RuntimeAvailable: false}}, HealthRuntimeUnavailable, false},
		{"unknown", []HealthObservation{{At: start, RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "unknown"}}, HealthUnknown, false},
		{"starting", []HealthObservation{{At: start, RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "starting"}}, HealthPending, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := EvaluateHealth(start, start.Add(time.Second), policy, tc.observations)
			if err != nil || result.Succeeded || result.Status != tc.want || result.Terminal != tc.terminal {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}

func TestEvaluateHealthConsecutiveFailuresAndReset(t *testing.T) {
	start, policy := healthFixture()
	observations := []HealthObservation{
		{At: start.Add(time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "unhealthy"},
		{At: start.Add(2 * time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "starting"},
		{At: start.Add(3 * time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "unhealthy"},
		{At: start.Add(4 * time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "unhealthy"},
	}
	result, err := EvaluateHealth(start, start.Add(5*time.Second), policy, observations)
	if err != nil || !result.Terminal || result.Succeeded || result.Status != HealthUnhealthy || result.ConsecutiveUnhealthy != 2 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestEvaluateHealthNewerValidObservationReplacesTransientRuntimeFailure(t *testing.T) {
	start, policy := healthFixture()
	observations := []HealthObservation{
		{At: start.Add(time.Second), RuntimeAvailable: false},
		{At: start.Add(2 * time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "starting"},
	}
	result, err := EvaluateHealth(start, start.Add(3*time.Second), policy, observations)
	if err != nil || result.Status != HealthPending || result.Terminal || result.InterventionRequired {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestEvaluateHealthTimeoutIsDeterministic(t *testing.T) {
	start, policy := healthFixture()
	result, err := EvaluateHealth(start, start.Add(policy.Timeout), policy, []HealthObservation{{At: start.Add(10 * time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "starting"}})
	if err != nil || result.Status != HealthTimedOut || !result.Terminal || !result.InterventionRequired || result.Succeeded {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestEvaluateHealthRejectsInvalidInput(t *testing.T) {
	start, policy := healthFixture()
	tests := []struct {
		name string
		p    HealthPolicy
		obs  []HealthObservation
		want string
	}{
		{"timeout", HealthPolicy{Timeout: 0, MaxConsecutiveUnhealthy: 2}, nil, "timeout"},
		{"failure limit", HealthPolicy{Timeout: time.Minute}, nil, "consecutive"},
		{"future observation", policy, []HealthObservation{{At: start.Add(2 * time.Second)}}, "timestamp"},
		{"out of order", policy, []HealthObservation{{At: start.Add(time.Second)}, {At: start}}, "timestamp"},
		{"unknown status", policy, []HealthObservation{{At: start, RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "mystery"}}, "unsupported"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EvaluateHealth(start, start.Add(time.Second), tc.p, tc.obs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}
