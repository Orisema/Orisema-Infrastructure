// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"strings"
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
