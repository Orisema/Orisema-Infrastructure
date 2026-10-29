// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, Operation, time.Time) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "operations")
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	op := Operation{ID: "op-123", Application: "demo", Services: []string{"web"}, Type: "UPDATE", CreatedAt: now}
	return store, op, now
}

func TestCreateLoadAndPersistenceAcrossReopen(t *testing.T) {
	store, op, now := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(op.ID, Preparing, "inspect", "ownership verified", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Load(op.ID)
	if err != nil || got.State != Preparing || len(got.Events) != 2 {
		t.Fatalf("got %#v, %v", got, err)
	}
	if info, err := os.Stat(filepath.Join(store.root, op.ID+".json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("unsafe journal mode: %v, %v", info, err)
	}
}

func TestRejectsInvalidTransitionAndCorruption(t *testing.T) {
	store, op, now := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(op.ID, Completed, "", "", now.Add(time.Second)); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid transition, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.root, op.ID+".json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil {
		t.Fatal("expected corrupt journal to fail")
	}
}

func TestLoadRejectsSymlinkAndBroadPermissions(t *testing.T) {
	store, op, _ := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, op.ID+".json")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected unsafe permissions error, got %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(store.root, "external.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil {
		t.Fatal("expected symlink journal to fail")
	}
}

func TestInterruptedTemporaryWriteDoesNotReplaceCommittedState(t *testing.T) {
	store, op, _ := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.root, ".operation-interrupted.tmp"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(op.ID)
	if err != nil || got.State != Pending {
		t.Fatalf("committed state lost: %#v, %v", got, err)
	}
}

func TestConcurrentTransitionAllowsOnlyOneWinner(t *testing.T) {
	store, op, now := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.Transition(op.ID, Preparing, "prepare", "", now.Add(time.Second))
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var success, rejected int
	for err := range errs {
		if err == nil {
			success++
		} else if strings.Contains(err.Error(), "invalid") {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("success=%d rejected=%d", success, rejected)
	}
}

func TestAssessmentNeverAuthorizesAutomaticResume(t *testing.T) {
	_, op, now := fixture(t)
	op.Version, op.State, op.UpdatedAt = Version, RecoveryRequired, now
	op.Events = []Event{{At: now, To: RecoveryRequired}}
	op.Previous, op.RecoveryData = "old-image", "container-id"
	a := Assess(op)
	if a.ResumeSafe || !a.Intervention || !a.RollbackCandidate {
		t.Fatalf("unsafe assessment: %#v", a)
	}
}
