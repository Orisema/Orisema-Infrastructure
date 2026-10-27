// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"strings"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

func TestEvaluateGuardrails(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	policy := &manifest.Recovery{MaxAttempts: 3, MinInterval: "5m", Cooldown: "30m"}
	attempt := func(minutes int) Attempt {
		return Attempt{Service: "web", Action: diagnostic.ActionStart, Timestamp: now.Add(time.Duration(minutes) * time.Minute)}
	}

	for _, tc := range []struct {
		name         string
		action       diagnostic.Action
		policy       *manifest.Recovery
		attempts     []Attempt
		allowed      bool
		reason       string
		retryMinutes int
	}{
		{"first attempt", diagnostic.ActionStart, policy, nil, true, "within declared", 0},
		{"policy required", diagnostic.ActionStart, nil, nil, false, "not declared", 0},
		{"invalid policy", diagnostic.ActionStart, &manifest.Recovery{}, nil, false, "invalid recovery", 0},
		{"only start", diagnostic.ActionRedeployReview, policy, nil, false, "not executable", 0},
		{"minimum interval", diagnostic.ActionStart, policy, []Attempt{attempt(-2)}, false, "minimum", 3},
		{"attempt limit", diagnostic.ActionStart, policy, []Attempt{attempt(-20), attempt(-10), attempt(-5)}, false, "limit reached", 10},
		{"old attempts expire", diagnostic.ActionStart, policy, []Attempt{attempt(-40), attempt(-35), attempt(-10)}, true, "within declared", 0},
		{"other service ignored", diagnostic.ActionStart, policy, []Attempt{{Service: "api", Action: diagnostic.ActionStart, Timestamp: now.Add(-time.Minute)}}, true, "within declared", 0},
		{"future history blocks", diagnostic.ActionStart, policy, []Attempt{attempt(1)}, false, "future timestamp", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(now, "web", tc.action, tc.policy, tc.attempts)
			if got.Allowed != tc.allowed || !strings.Contains(got.Reason, tc.reason) {
				t.Fatalf("got %+v, want allowed=%t reason containing %q", got, tc.allowed, tc.reason)
			}
			if tc.retryMinutes > 0 && !got.RetryAfter.Equal(now.Add(time.Duration(tc.retryMinutes)*time.Minute)) {
				t.Fatalf("retry after = %v, want %v", got.RetryAfter, now.Add(time.Duration(tc.retryMinutes)*time.Minute))
			}
		})
	}
}
