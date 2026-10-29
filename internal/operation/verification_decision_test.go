// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func decisionFixture(t *testing.T, status string, failures int) (Operation, []Resource) {
	t.Helper()
	store, op, now := fixture(t)
	op.Resources = resourceFixture()
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	for i, state := range []State{Preparing, Applying} {
		if _, err := store.Transition(op.ID, state, "", "", now.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	policy := HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: failures}
	started := now.Add(3 * time.Second)
	if _, err := store.StartHealthVerification(op.ID, policy, started); err != nil {
		t.Fatal(err)
	}
	if status != "" {
		observation := HealthObservation{At: started.Add(time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: status}
		if _, err := store.RecordHealthObservation(op.ID, observation, observation.At); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := store.Load(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded, append([]Resource(nil), op.Resources...)
}

func TestDecisionWaitsForTerminalHealth(t *testing.T) {
	op, observed := decisionFixture(t, "", 2)
	decision := DecideAfterVerification(op, observed)
	if decision.Action != VerificationWait || decision.TransitionAllowed || decision.InterventionRequired {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestDecisionAllowsCompleteOnlyWithHealthyAndFreshResources(t *testing.T) {
	op, observed := decisionFixture(t, "healthy", 2)
	before := op
	decision := DecideAfterVerification(op, observed)
	if decision.Action != VerificationComplete || decision.TargetState != Completed || !decision.TransitionAllowed || decision.InterventionRequired || !decision.ResourceEvidence.Safe {
		t.Fatalf("decision=%#v", decision)
	}
	if !reflect.DeepEqual(before, op) {
		t.Fatal("pure decision mutated operation")
	}
}

func TestDecisionRequiresRecoveryForTerminalFailure(t *testing.T) {
	op, observed := decisionFixture(t, "unhealthy", 1)
	decision := DecideAfterVerification(op, observed)
	if decision.Action != VerificationRecoveryRequired || decision.TargetState != RecoveryRequired || !decision.TransitionAllowed || !decision.InterventionRequired {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestDecisionBlocksResourceDivergence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]Resource) []Resource
		want   string
	}{
		{"missing", func(r []Resource) []Resource { return r[:1] }, "missing"},
		{"identity", func(r []Resource) []Resource { r[0].ID = "changed"; return r }, "identity"},
		{"ownership", func(r []Resource) []Resource { r[0].Ownership = OwnershipUnknown; return r }, "ownership"},
		{"duplicate", func(r []Resource) []Resource { return append(r, r[0]) }, "duplicate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op, observed := decisionFixture(t, "healthy", 2)
			decision := DecideAfterVerification(op, tc.mutate(observed))
			if decision.Action != VerificationBlocked || decision.TransitionAllowed || !decision.InterventionRequired || !strings.Contains(decision.Reason, tc.want) {
				t.Fatalf("decision=%#v", decision)
			}
		})
	}
}

func TestDecisionBlocksInconsistentStateAndUnknownRecordedOwnership(t *testing.T) {
	op, observed := decisionFixture(t, "healthy", 2)
	op.State = Applying
	if decision := DecideAfterVerification(op, observed); decision.Action != VerificationBlocked {
		t.Fatalf("state decision=%#v", decision)
	}
	op, observed = decisionFixture(t, "healthy", 2)
	op.Resources[0].Ownership = OwnershipUnknown
	if decision := DecideAfterVerification(op, observed); decision.Action != VerificationBlocked || !strings.Contains(decision.Reason, "incomplete or ambiguous") {
		t.Fatalf("ownership decision=%#v", decision)
	}
}
