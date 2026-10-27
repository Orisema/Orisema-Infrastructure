// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"fmt"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

// Attempt is the minimum auditable record required to evaluate recovery
// guardrails. No CLI command writes attempts yet.
type Attempt struct {
	Service   string
	Action    diagnostic.Action
	Timestamp time.Time
	Succeeded bool
}

type Decision struct {
	Allowed    bool
	Reason     string
	RetryAfter time.Time
}

// Evaluate applies a rolling cooldown window and minimum interval. It only
// evaluates START because no other mutating recovery action is implemented.
func Evaluate(now time.Time, service string, action diagnostic.Action, policy *manifest.Recovery, attempts []Attempt) Decision {
	if action != diagnostic.ActionStart {
		return Decision{Reason: "action is not executable by recovery"}
	}
	if policy == nil {
		return Decision{Reason: "recovery guardrails are not declared"}
	}
	if policy.MaxAttempts < 1 || policy.MaxAttempts > 10 {
		return Decision{Reason: "invalid recovery maxAttempts"}
	}
	minInterval, err := time.ParseDuration(policy.MinInterval)
	if err != nil || minInterval < time.Second || minInterval > 24*time.Hour {
		return Decision{Reason: "invalid recovery minInterval"}
	}
	cooldown, err := time.ParseDuration(policy.Cooldown)
	if err != nil || cooldown < minInterval || cooldown > 7*24*time.Hour {
		return Decision{Reason: "invalid recovery cooldown"}
	}

	matching := make([]Attempt, 0, len(attempts))
	var latest time.Time
	for _, attempt := range attempts {
		if attempt.Service != service || attempt.Action != action {
			continue
		}
		if attempt.Timestamp.After(now) {
			return Decision{Reason: "recovery history contains a future timestamp"}
		}
		if attempt.Timestamp.After(now.Add(-cooldown)) || attempt.Timestamp.Equal(now.Add(-cooldown)) {
			matching = append(matching, attempt)
		}
		if attempt.Timestamp.After(latest) {
			latest = attempt.Timestamp
		}
	}
	if len(matching) >= policy.MaxAttempts {
		earliest := matching[0].Timestamp
		for _, attempt := range matching[1:] {
			if attempt.Timestamp.Before(earliest) {
				earliest = attempt.Timestamp
			}
		}
		return Decision{Reason: fmt.Sprintf("recovery attempt limit reached (%d in cooldown window)", len(matching)), RetryAfter: earliest.Add(cooldown)}
	}
	if !latest.IsZero() && now.Before(latest.Add(minInterval)) {
		return Decision{Reason: "minimum recovery interval has not elapsed", RetryAfter: latest.Add(minInterval)}
	}
	return Decision{Allowed: true, Reason: "within declared guardrails; explicit confirmation is still required"}
}

func PolicyFor(m *manifest.Manifest, service string) *manifest.Recovery {
	for i := range m.Services {
		if m.Services[i].Name == service {
			return m.Services[i].Recovery
		}
	}
	return nil
}
