// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

func committableDecisionFixture(t *testing.T, status string, failures int) (*Store, Operation, []Resource, time.Time) {
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
	started := now.Add(3 * time.Second)
	if _, err := store.StartHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: failures}, started); err != nil {
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
	return store, loaded, append([]Resource(nil), op.Resources...), started.Add(2 * time.Second)
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

func TestCommitVerificationDecisionPersistsCompleteAndReopens(t *testing.T) {
	store, op, observed, at := committableDecisionFixture(t, "healthy", 2)
	got, err := store.CommitVerificationDecision(op.ID, observed, at)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Completed || got.Decision == nil || got.Decision.Action != VerificationComplete || !reflect.DeepEqual(got.Decision.ObservedResources, observed) {
		t.Fatalf("operation=%#v", got)
	}
	reopened, err := OpenStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(op.ID)
	if err != nil || !reflect.DeepEqual(loaded, got) {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	if _, err := reopened.CommitVerificationDecision(op.ID, observed, at.Add(time.Second)); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("expected duplicate decision rejection, got %v", err)
	}
}

func TestCommitVerificationDecisionPersistsRecoveryRequirement(t *testing.T) {
	store, op, observed, at := committableDecisionFixture(t, "unhealthy", 1)
	got, err := store.CommitVerificationDecision(op.ID, observed, at)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != RecoveryRequired || got.Decision == nil || got.Decision.Action != VerificationRecoveryRequired || got.Failure != got.Decision.Reason {
		t.Fatalf("operation=%#v", got)
	}
}

func TestCommitVerificationDecisionWaitAndBlockedDoNotWrite(t *testing.T) {
	tests := []struct {
		name, status string
		mutate       func([]Resource) []Resource
	}{
		{name: "wait"},
		{name: "blocked", status: "healthy", mutate: func(resources []Resource) []Resource { return resources[:1] }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, op, observed, at := committableDecisionFixture(t, tc.status, 2)
			if tc.mutate != nil {
				observed = tc.mutate(observed)
			}
			before, err := os.ReadFile(filepath.Join(store.root, op.ID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.CommitVerificationDecision(op.ID, observed, at); err == nil {
				t.Fatal("expected non-committable decision")
			}
			after, err := os.ReadFile(filepath.Join(store.root, op.ID+".json"))
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("journal changed: %v", err)
			}
		})
	}
}

func TestConcurrentCommitVerificationDecisionHasOneWinner(t *testing.T) {
	store, op, observed, at := committableDecisionFixture(t, "healthy", 2)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.CommitVerificationDecision(op.ID, observed, at)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var successes, rejected int
	for err := range errs {
		if err == nil {
			successes++
		} else if strings.Contains(err.Error(), "already") {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("successes=%d rejected=%d", successes, rejected)
	}
}

func TestLoadRejectsTamperedPersistedDecision(t *testing.T) {
	store, op, observed, at := committableDecisionFixture(t, "healthy", 2)
	if _, err := store.CommitVerificationDecision(op.ID, observed, at); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, op.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["decision"].(map[string]any)["reason"] = "tampered"
	data, _ = json.Marshal(raw)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil || !strings.Contains(err.Error(), "matching transition") {
		t.Fatalf("expected tampering rejection, got %v", err)
	}
}
