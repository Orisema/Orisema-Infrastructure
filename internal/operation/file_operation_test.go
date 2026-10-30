// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileOperationPersistsOrderedBoundaries(t *testing.T) {
	store, _, now := fixture(t)
	op, err := store.CreateFileOperation(FileOperationRequest{ID: "backup-op-test", Type: "BACKUP", Application: "demo", BackupID: "backup-test", Source: "/safe/manifest", SourceSHA256: strings.Repeat("a", 64), Artifact: "/safe/backup", Staging: "/safe/backup.staging-test", FirstStep: "stage", At: now})
	if err != nil || op.State != Preparing {
		t.Fatalf("op=%#v err=%v", op, err)
	}
	steps := []struct {
		done, next string
		state      State
	}{{"stage", "payload", Applying}, {"payload", "metadata", Applying}, {"metadata", "publish", Applying}, {"publish", "verify", Verifying}, {"verify", "", Completed}}
	for i, step := range steps {
		op, err = store.RecordFileOperationStep(op.ID, step.done, step.next, now.Add(time.Duration(i+1)*time.Second))
		if err != nil || op.State != step.state || op.FileOperation.PendingStep != step.next {
			t.Fatalf("step=%s op=%#v err=%v", step.done, op, err)
		}
		loaded, loadErr := store.Load(op.ID)
		if loadErr != nil || loaded.FileOperation.PendingStep != step.next {
			t.Fatalf("persisted step=%s loaded=%#v err=%v", step.done, loaded, loadErr)
		}
		if step.state != Completed {
			assessment := Assess(loaded)
			if !assessment.Intervention || assessment.ResumeSafe || !strings.Contains(assessment.Reason, "staging") {
				t.Fatalf("assessment=%#v", assessment)
			}
		}
	}
}

func TestFileOperationRejectsOrderReplayAndTampering(t *testing.T) {
	store, _, now := fixture(t)
	op, err := store.CreateFileOperation(FileOperationRequest{ID: "restore-op-test", Type: "RESTORE", Application: "demo", BackupID: "backup-test", Source: "/a", Artifact: "/a", Destination: "/b", Staging: "/b.staging-test", FirstStep: "stage", At: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordFileOperationStep(op.ID, "payload", "publish", now.Add(time.Second)); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("err=%v", err)
	}
	if _, err := store.RecordFileOperationStep(op.ID, "stage", "payload", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordFileOperationStep(op.ID, "stage", "payload", now.Add(2*time.Second)); err == nil {
		t.Fatal("replay succeeded")
	}
	loaded, _ := store.Load(op.ID)
	loaded.FileOperation.PendingStep = "unknown"
	path, _ := store.path(op.ID)
	if err := store.atomicWrite(path, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil {
		t.Fatal("tampered journal loaded")
	}
}

func TestConcurrentFileOperationsForApplicationHaveOneWinner(t *testing.T) {
	store, _, now := fixture(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id := "backup-concurrent-" + string(rune('a'+i))
			_, err := store.CreateFileOperation(FileOperationRequest{ID: id, Type: "BACKUP", Application: "demo", BackupID: "backup", Source: "/manifest", SourceSHA256: strings.Repeat("a", 64), Artifact: "/backup", Staging: "/backup.staging-" + strings.TrimPrefix(id, "backup-op-"), FirstStep: "stage", At: now})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	success, busy := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if strings.Contains(err.Error(), ErrApplicationBusy.Error()) {
			busy++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || busy != 1 {
		t.Fatalf("success=%d busy=%d", success, busy)
	}
}
