// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
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

func TestExistingStoreModesDoNotCreateOrEscalate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "operations")
	if _, err := OpenExistingWritableStore(root); err == nil {
		t.Fatal("writable open created or accepted a missing store")
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("existing-store open changed the filesystem: %v", err)
	}

	initialized, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := OpenExistingStore(root)
	if err != nil {
		t.Fatal(err)
	}
	op := Operation{ID: "read-only", Application: "demo", Services: []string{"web"}, Type: "START"}
	if err := readOnly.Create(op); !errors.Is(err, ErrReadOnlyStore) {
		t.Fatalf("read-only store mutation error=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, op.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only store created a snapshot: %v", err)
	}

	writable, err := OpenExistingWritableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writable.Create(op); err != nil {
		t.Fatal(err)
	}
	if _, err := initialized.Load(op.ID); err != nil {
		t.Fatalf("admitted operation was not persisted: %v", err)
	}
}

func TestInitializeStoreCreatesOnceAndDoesNotRepair(t *testing.T) {
	root := filepath.Join(t.TempDir(), "operations")
	if _, err := InitializeStore(root); err != nil {
		t.Fatal(err)
	}
	if _, err := InitializeStore(root); err != nil {
		t.Fatalf("safe initialization is not idempotent: %v", err)
	}
	if err := os.Chmod(filepath.Join(root, ".store.lock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InitializeStore(root); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("initialization repaired unsafe state: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, ".store.lock"))
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("initialization changed unsafe lock: %v, %v", info, err)
	}
}

func TestControlIntentRejectsTampering(t *testing.T) {
	store, _, _ := fixture(t)
	intent := &ControlIntent{
		Contract: ControlContract, Action: "START", ResourceName: "orisema-demo-web", ResourceID: "container-1",
		InitialRunning: false, ExpectedRunning: true,
	}
	intent.PlanFingerprint = ControlPlanFingerprint("demo", "web", "START", intent.ResourceName, intent.ResourceID, false, true)
	op := Operation{
		ID: "start-1", Application: "demo", Services: []string{"web"}, Type: "START", Control: intent,
		Resources: []Resource{{Kind: "container", Name: intent.ResourceName, ID: intent.ResourceID, Ownership: OwnershipManaged, Application: "demo", Service: "web"}},
	}
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, op.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), `"expectedRunning": true`, `"expectedRunning": false`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(op.ID); err == nil || !strings.Contains(err.Error(), "intent") {
		t.Fatalf("tampered control intent error=%v", err)
	}
}

func TestExistingWritableStoreRejectsUnsafeLockWithoutRepair(t *testing.T) {
	root := filepath.Join(t.TempDir(), "operations")
	if _, err := OpenStore(root); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, ".store.lock")
	if err := os.Chmod(lockPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExistingWritableStore(root); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe writable store error=%v", err)
	}
	info, err := os.Stat(lockPath)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("writable open repaired unsafe lock: %v, %v", info, err)
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

func TestCheckContinuationAllowsOnlyExactIncompleteOperation(t *testing.T) {
	store, op, _ := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckContinuation("demo", op.ID); err != nil {
		t.Fatalf("exact continuation blocked: %v", err)
	}
	if err := store.CheckContinuation("demo", "different"); !errors.Is(err, ErrApplicationBusy) {
		t.Fatalf("conflicting continuation error=%v", err)
	}
	if err := store.CheckContinuation("other", op.ID); !errors.Is(err, ErrContinuationBlocked) {
		t.Fatalf("wrong application error=%v", err)
	}
}

func TestCheckContinuationFailsClosedOnOtherCorruptSnapshot(t *testing.T) {
	store, op, _ := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.root, "other.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckContinuation("demo", op.ID); err == nil || !strings.Contains(err.Error(), "validate operation store") {
		t.Fatalf("corrupt store did not block continuation: %v", err)
	}
}

func TestAssessmentNeverAuthorizesAutomaticResume(t *testing.T) {
	_, op, now := fixture(t)
	op.Version, op.State, op.UpdatedAt = Version, RecoveryRequired, now
	op.Events = []Event{{At: now, To: RecoveryRequired}}
	op.Previous, op.RecoveryData = "old-image", "container-id"
	op.Resources = []Resource{{Kind: "container", Name: "orisema-demo-web", ID: "container-id", Ownership: OwnershipManaged, Application: "demo", Service: "web"}}
	a := Assess(op)
	if a.ResumeSafe || !a.Intervention || !a.RollbackCandidate {
		t.Fatalf("unsafe assessment: %#v", a)
	}
}

func TestConcurrentCreateAllowsOneActiveOperationPerApplication(t *testing.T) {
	store, op, now := fixture(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, id := range []string{"op-a", "op-b"} {
		wg.Add(1)
		go func(id string, offset int) {
			defer wg.Done()
			<-start
			candidate := op
			candidate.ID = id
			candidate.CreatedAt = now.Add(time.Duration(offset) * time.Second)
			errs <- store.Create(candidate)
		}(id, i)
	}
	close(start)
	wg.Wait()
	close(errs)
	var success, busy int
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrApplicationBusy) {
			busy++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || busy != 1 {
		t.Fatalf("success=%d busy=%d", success, busy)
	}
}

func TestDifferentApplicationsAndTerminalRelease(t *testing.T) {
	store, op, now := fixture(t)
	if err := store.Create(op); err != nil {
		t.Fatal(err)
	}
	other := op
	other.ID, other.Application = "op-other", "other"
	if err := store.Create(other); err != nil {
		t.Fatalf("different application blocked: %v", err)
	}
	states := []State{Preparing, Applying, Verifying, Completed}
	for i, state := range states {
		if _, err := store.Transition(op.ID, state, "", "", now.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	next := op
	next.ID, next.CreatedAt = "op-next", now.Add(10*time.Second)
	if err := store.Create(next); err != nil {
		t.Fatalf("terminal operation did not release application: %v", err)
	}
	incomplete, err := store.Incomplete()
	if err != nil {
		t.Fatal(err)
	}
	if len(incomplete) != 2 || incomplete[0].ID != "op-other" || incomplete[1].ID != "op-next" {
		t.Fatalf("unexpected deterministic incomplete list: %#v", incomplete)
	}
}

func TestConflictCheckFailsClosedOnCorruptJournal(t *testing.T) {
	store, op, _ := fixture(t)
	if err := os.WriteFile(filepath.Join(store.root, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(op); err == nil || !strings.Contains(err.Error(), "check concurrent") {
		t.Fatalf("expected corrupt journal to block create, got %v", err)
	}
	if operations, err := store.List(); err == nil || operations != nil {
		t.Fatalf("expected corrupt list to fail closed, got %#v, %v", operations, err)
	}
}
