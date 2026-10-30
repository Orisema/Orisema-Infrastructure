// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"strings"
	"testing"
	"time"
)

func recoveryRequiredUpdate(t *testing.T) (*Store, Operation, time.Time, []Resource) {
	t.Helper()
	store, op, now, resources := completedUpdateExecution(t)
	started := now.Add(3 * time.Minute)
	if _, err := store.StartUpdateHealthVerification(op.ID, HealthPolicy{Timeout: time.Minute, MaxConsecutiveUnhealthy: 1}, resources, started); err != nil {
		t.Fatal(err)
	}
	at := started.Add(time.Second)
	if _, err := store.RecordHealthObservation(op.ID, HealthObservation{At: at, RuntimeAvailable: true, Running: false, HealthcheckPresent: true, Status: "unknown"}, at); err != nil {
		t.Fatal(err)
	}
	op, err := store.CommitVerificationDecision(op.ID, resources, at.Add(time.Second))
	if err != nil || op.State != RecoveryRequired {
		t.Fatalf("operation=%#v err=%v", op, err)
	}
	return store, op, now, resources
}

func TestUpdateRollbackPersistsEveryBoundaryAndHealthyDecision(t *testing.T) {
	store, op, now, resources := recoveryRequiredUpdate(t)
	failed := "orisema-failed-candidate"
	if _, err := store.BeginUpdateRollback(op.ID, failed, resources, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateRollbackStep(op.ID, "candidate-stopped", now.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateRollbackStep(op.ID, "rename-candidate", now.Add(7*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateRollbackStep(op.ID, "candidate-renamed", now.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateRollbackStep(op.ID, "rename-previous", now.Add(9*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateRollbackStep(op.ID, "previous-renamed", now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateRollbackStep(op.ID, "start-previous", now.Add(11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordUpdateRollbackStep(op.ID, "previous-started", now.Add(12*time.Minute)); err != nil {
		t.Fatal(err)
	}
	restored := []Resource{
		{Kind: "container", Name: failed, ID: "candidate-456", Ownership: OwnershipManaged, Application: "demo", Service: "web"},
		{Kind: "container", Name: "orisema-demo-web", ID: "container-123", Ownership: OwnershipManaged, Application: "demo", Service: "web"},
	}
	waiting, err := store.RecordUpdateRollbackHealth(op.ID, restored, "starting", now.Add(13*time.Minute))
	if err != nil || waiting.State != RollingBack {
		t.Fatalf("operation=%#v err=%v", waiting, err)
	}
	done, err := store.RecordUpdateRollbackHealth(op.ID, restored, "healthy", now.Add(14*time.Minute))
	if err != nil || done.State != RolledBack || done.Rollback.HealthStatus != "healthy" {
		t.Fatalf("operation=%#v err=%v", done, err)
	}
}

func TestUpdateRollbackFailsClosedOnDriftOrderAndReplay(t *testing.T) {
	store, op, now, resources := recoveryRequiredUpdate(t)
	changed := append([]Resource(nil), resources...)
	changed[0].ID = "changed"
	if _, err := store.BeginUpdateRollback(op.ID, "orisema-failed-drift", changed, now.Add(5*time.Minute)); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("err=%v", err)
	}
	if _, err := store.BeginUpdateRollback(op.ID, "orisema-failed-order", resources, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUpdateRollbackStep(op.ID, "rename-candidate", now.Add(6*time.Minute)); err == nil {
		t.Fatal("out-of-order prepare succeeded")
	}
	if _, err := store.BeginUpdateRollback(op.ID, "orisema-failed-replay", resources, now.Add(6*time.Minute)); err == nil {
		t.Fatal("replayed rollback succeeded")
	}
}

func TestUpdateRollbackTamperingFailsClosed(t *testing.T) {
	store, op, now, resources := recoveryRequiredUpdate(t)
	started, err := store.BeginUpdateRollback(op.ID, "orisema-failed-tamper", resources, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	started.Rollback.PreviousStarted = true
	path, _ := store.path(op.ID)
	if err := store.atomicWrite(path, started); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("err=%v", err)
	}
}
