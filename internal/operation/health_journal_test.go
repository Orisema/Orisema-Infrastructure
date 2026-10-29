// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"strings"
	"testing"
	"time"
)

func verifyingFixture(t *testing.T) (*Store, Operation, time.Time, HealthPolicy) {
	t.Helper()
	store, op, now := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	for i, state := range []State{Preparing, Applying} {
		if _, err := store.Transition(op.ID, state, "", "", now.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	policy := HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 2}
	started, err := store.StartHealthVerification(op.ID, policy, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return store, started, now.Add(3 * time.Second), policy
}

func TestPersistedHealthVerificationSurvivesReopen(t *testing.T) {
	store, op, started, _ := verifyingFixture(t)
	observation := HealthObservation{At: started.Add(time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "healthy"}
	updated, err := store.RecordHealthObservation(op.ID, observation, observation.At)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Health.Evaluation.Succeeded || !updated.Health.Evaluation.Terminal {
		t.Fatalf("evaluation=%#v", updated.Health.Evaluation)
	}
	reopened, err := OpenExistingStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(op.ID)
	if err != nil || len(loaded.Health.Observations) != 1 || loaded.Health.Evaluation.Status != HealthHealthy {
		t.Fatalf("loaded=%#v err=%v", loaded.Health, err)
	}
	if _, err := store.RecordHealthObservation(op.ID, observation, observation.At.Add(time.Second)); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("expected terminal rejection, got %v", err)
	}
}

func TestRefreshHealthVerificationPersistsTimeout(t *testing.T) {
	store, op, started, policy := verifyingFixture(t)
	starting := HealthObservation{At: started.Add(time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "starting"}
	if _, err := store.RecordHealthObservation(op.ID, starting, starting.At); err != nil {
		t.Fatal(err)
	}
	updated, err := store.RefreshHealthVerification(op.ID, started.Add(policy.Timeout))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Health.Evaluation.Status != HealthTimedOut || !updated.Health.Evaluation.Terminal || len(updated.Health.Observations) != 1 {
		t.Fatalf("health=%#v", updated.Health)
	}
}

func TestPersistedHealthEvaluationMismatchFailsClosed(t *testing.T) {
	store, op, _, _ := verifyingFixture(t)
	loaded, err := store.Load(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Health.Evaluation.Status = HealthHealthy
	path, _ := store.path(op.ID)
	if err := store.atomicWrite(path, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}

func TestHealthVerificationRequiresApplyingState(t *testing.T) {
	store, op, now := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	_, err := store.StartHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 2}, now.Add(time.Second))
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestLoadRejectsHealthBeforeVerifyingState(t *testing.T) {
	store, op, now := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	policy := HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 2}
	evaluation, err := EvaluateHealth(now, now, policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Health = &HealthVerification{StartedAt: now, Policy: policy, Evaluation: evaluation}
	path, _ := store.path(op.ID)
	if err := store.atomicWrite(path, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil || !strings.Contains(err.Error(), "before") {
		t.Fatalf("expected impossible health state rejection, got %v", err)
	}
}
