// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func authorizedIntent(t *testing.T) (*Store, Operation, time.Time) {
	t.Helper()
	store, _, now := fixture(t)
	created, err := store.CreateUpdateIntent(intentRequest("update-execution", now))
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := store.AuthorizeUpdateIntent(created.ID, created.Intent.PlanFingerprint, observedIntentResource(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return store, authorized, now
}

func TestUpdateExecutionPersistsOrderedRuntimeEvidence(t *testing.T) {
	store, authorized, now := authorizedIntent(t)
	backup := "orisema-rollback-123"
	applying, err := store.BeginUpdateExecution(authorized.ID, authorized.Intent.PlanFingerprint, backup, observedIntentResource(), now.Add(2*time.Minute))
	if err != nil || applying.State != Applying || applying.Execution.BackupName != backup {
		t.Fatalf("applying=%#v err=%v", applying, err)
	}
	if _, err := store.RecordUpdateExecutionStep(authorized.ID, "previous-stopped", "", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateExecutionStep(authorized.ID, "rename-previous", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateExecutionStep(authorized.ID, "previous-renamed", "", now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateExecutionStep(authorized.ID, "create-candidate", now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := store.RecordUpdateExecutionStep(authorized.ID, "candidate-created", "candidate-456", now.Add(5*time.Minute))
	if err != nil || got.Execution.CandidateID != "candidate-456" || !got.Execution.PreviousStopped || !got.Execution.PreviousRenamed {
		t.Fatalf("operation=%#v err=%v", got, err)
	}
	loaded, err := store.Load(authorized.ID)
	if err != nil || loaded.Execution.CandidateID != "candidate-456" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestUpdateExecutionRejectsStaleDivergentAndRepeatedStarts(t *testing.T) {
	t.Run("stale", func(t *testing.T) {
		store, op, now := authorizedIntent(t)
		_, err := store.BeginUpdateExecution(op.ID, op.Intent.PlanFingerprint, "orisema-rollback-stale", observedIntentResource(), now.Add(7*time.Minute))
		if err == nil || !strings.Contains(err.Error(), "stale") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("divergent", func(t *testing.T) {
		store, op, now := authorizedIntent(t)
		changed := observedIntentResource()
		changed[0].ID = "changed"
		_, err := store.BeginUpdateExecution(op.ID, op.Intent.PlanFingerprint, "orisema-rollback-divergent", changed, now.Add(2*time.Minute))
		if err == nil || !strings.Contains(err.Error(), "changed") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("repeated", func(t *testing.T) {
		store, op, now := authorizedIntent(t)
		if _, err := store.BeginUpdateExecution(op.ID, op.Intent.PlanFingerprint, "orisema-rollback-repeat", observedIntentResource(), now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
		_, err := store.BeginUpdateExecution(op.ID, op.Intent.PlanFingerprint, "orisema-rollback-repeat", observedIntentResource(), now.Add(2*time.Minute))
		if err == nil || !strings.Contains(err.Error(), "not ready") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestUpdateExecutionStepOrderFailsClosed(t *testing.T) {
	store, op, now := authorizedIntent(t)
	if _, err := store.BeginUpdateExecution(op.ID, op.Intent.PlanFingerprint, "orisema-rollback-order", observedIntentResource(), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateExecutionStep(op.ID, "previous-renamed", "", now.Add(3*time.Minute)); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("err=%v", err)
	}
}

func completedUpdateExecution(t *testing.T) (*Store, Operation, time.Time, []Resource) {
	t.Helper()
	store, op, now := authorizedIntent(t)
	backup := "orisema-rollback-health"
	if _, err := store.BeginUpdateExecution(op.ID, op.Intent.PlanFingerprint, backup, observedIntentResource(), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateExecutionStep(op.ID, "previous-stopped", "", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateExecutionStep(op.ID, "rename-previous", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateExecutionStep(op.ID, "previous-renamed", "", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateExecutionStep(op.ID, "create-candidate", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateExecutionStep(op.ID, "candidate-created", "candidate-456", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	resources := []Resource{
		{Kind: "container", Name: "orisema-demo-web", ID: "candidate-456", Ownership: OwnershipManaged, Application: "demo", Service: "web"},
		{Kind: "container", Name: backup, ID: "container-123", Ownership: OwnershipManaged, Application: "demo", Service: "web"},
	}
	loaded, err := store.Load(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	return store, loaded, now, resources
}

func TestUpdateHealthVerificationBindsCandidateAndPreviousResources(t *testing.T) {
	store, op, now, resources := completedUpdateExecution(t)
	started, err := store.StartUpdateHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 3}, resources, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if started.State != Verifying || len(started.Health.VerificationResources) != 2 {
		t.Fatalf("operation=%#v", started)
	}
	observation := HealthObservation{At: now.Add(3*time.Minute + time.Second), RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "healthy"}
	if _, err := store.RecordHealthObservation(op.ID, observation, observation.At); err != nil {
		t.Fatal(err)
	}
	completed, err := store.CommitVerificationDecision(op.ID, resources, observation.At.Add(time.Second))
	if err != nil || completed.State != Completed || completed.Decision.Action != VerificationComplete {
		t.Fatalf("operation=%#v err=%v", completed, err)
	}
}

func TestUpdateHealthVerificationRejectsChangedRuntimeEvidence(t *testing.T) {
	store, op, now, resources := completedUpdateExecution(t)
	resources[0].ID = "replaced-candidate"
	before, err := store.Load(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.StartUpdateHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 3}, resources, now.Add(3*time.Minute))
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("err=%v", err)
	}
	after, _ := store.Load(op.ID)
	if after.State != before.State || after.Health != nil {
		t.Fatal("failed verification start changed the journal")
	}
}

func TestUpdateHealthFailurePersistsRecoveryRequired(t *testing.T) {
	store, op, now, resources := completedUpdateExecution(t)
	started := now.Add(3 * time.Minute)
	if _, err := store.StartUpdateHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 2}, resources, started); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		at := started.Add(time.Duration(i) * time.Second)
		if _, err := store.RecordHealthObservation(op.ID, HealthObservation{At: at, RuntimeAvailable: true, Running: true, HealthcheckPresent: true, Status: "unhealthy"}, at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.CommitVerificationDecision(op.ID, resources, started.Add(3*time.Second))
	if err != nil || got.State != RecoveryRequired || got.Decision.Action != VerificationRecoveryRequired || got.Failure == "" {
		t.Fatalf("operation=%#v err=%v", got, err)
	}
}

func TestUpdateHealthVerificationTamperingFailsClosed(t *testing.T) {
	store, op, now, resources := completedUpdateExecution(t)
	started, err := store.StartUpdateHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 2}, resources, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	started.Health.VerificationResources[0].ID = "tampered"
	path, _ := store.path(op.ID)
	if err := store.atomicWrite(path, started); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("err=%v", err)
	}
}

func TestConcurrentUpdateHealthVerificationHasOneWinner(t *testing.T) {
	store, op, now, resources := completedUpdateExecution(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.StartUpdateHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 2}, resources, now.Add(3*time.Minute))
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
		} else if strings.Contains(err.Error(), "already been started") {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("successes=%d rejected=%d", successes, rejected)
	}
}
