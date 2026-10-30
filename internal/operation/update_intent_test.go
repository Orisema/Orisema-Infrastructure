// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func intentRequest(id string, at time.Time) UpdateIntentRequest {
	return UpdateIntentRequest{
		ID: id, Application: "demo", Service: "web", ResourceName: "orisema-demo-web",
		ResourceID: "container-123", CurrentImage: "nginx:old", TargetImage: "nginx:new", At: at,
	}
}

func TestCreateUpdateIntentPersistsAndReopens(t *testing.T) {
	store, _, now := fixture(t)
	got, err := store.CreateUpdateIntent(intentRequest("intent-1", now))
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Pending || got.Intent == nil || got.Intent.PlanFingerprint == "" || got.Previous != "nginx:old" || got.Resources[0].ID != "container-123" {
		t.Fatalf("operation=%#v", got)
	}
	reopened, err := OpenExistingStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(got.ID)
	if err != nil || loaded.Intent.PlanFingerprint != got.Intent.PlanFingerprint {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestConcurrentUpdateIntentAllowsOnePerApplication(t *testing.T) {
	store, _, now := fixture(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"intent-a", "intent-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			_, err := store.CreateUpdateIntent(intentRequest(id, now))
			errs <- err
		}(id)
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

func TestLoadRejectsTamperedUpdateIntent(t *testing.T) {
	store, _, now := fixture(t)
	got, err := store.CreateUpdateIntent(intentRequest("intent-tamper", now))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, got.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["intent"].(map[string]any)["targetImage"] = "attacker/image"
	data, _ = json.Marshal(raw)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(got.ID); err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("expected tampering rejection, got %v", err)
	}
}

func TestCreateUpdateIntentRejectsIncompleteEvidence(t *testing.T) {
	store, _, now := fixture(t)
	request := intentRequest("intent-invalid", now)
	request.ResourceID = ""
	if _, err := store.CreateUpdateIntent(request); err == nil || !strings.Contains(err.Error(), "managed") {
		t.Fatalf("expected incomplete evidence rejection, got %v", err)
	}
}

func observedIntentResource() []Resource {
	return []Resource{{Kind: "container", Name: "orisema-demo-web", ID: "container-123", Ownership: OwnershipManaged, Application: "demo", Service: "web"}}
}

func TestAuthorizeUpdateIntentPersistsRevalidatedEvidence(t *testing.T) {
	store, _, now := fixture(t)
	created, err := store.CreateUpdateIntent(intentRequest("intent-authorize", now))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.AuthorizeUpdateIntent(created.ID, created.Intent.PlanFingerprint, observedIntentResource(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Preparing || got.Authorization == nil || !got.Authorization.ResourceEvidence.Safe {
		t.Fatalf("operation=%#v", got)
	}
	loaded, err := store.Load(got.ID)
	if err != nil || loaded.Authorization == nil {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestAuthorizeUpdateIntentFailuresDoNotWrite(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint func(Operation) string
		observed    []Resource
		offset      time.Duration
		want        string
	}{
		{"expired", func(op Operation) string { return op.Intent.PlanFingerprint }, observedIntentResource(), UpdateAuthorizationTTL + time.Second, "expired"},
		{"plan changed", func(Operation) string { return strings.Repeat("0", 64) }, observedIntentResource(), time.Minute, "fingerprint"},
		{"identity changed", func(op Operation) string { return op.Intent.PlanFingerprint }, []Resource{{Kind: "container", Name: "orisema-demo-web", ID: "changed", Ownership: OwnershipManaged, Application: "demo", Service: "web"}}, time.Minute, "identity"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, now := fixture(t)
			created, err := store.CreateUpdateIntent(intentRequest("intent-failure", now))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(store.root, created.ID+".json")
			before, _ := os.ReadFile(path)
			_, err = store.AuthorizeUpdateIntent(created.ID, tc.fingerprint(created), tc.observed, now.Add(tc.offset))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("failed authorization changed journal")
			}
		})
	}
}

func TestConcurrentAuthorizeUpdateIntentHasOneWinner(t *testing.T) {
	store, _, now := fixture(t)
	created, err := store.CreateUpdateIntent(intentRequest("intent-concurrent-auth", now))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.AuthorizeUpdateIntent(created.ID, created.Intent.PlanFingerprint, observedIntentResource(), now.Add(time.Minute))
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
		} else if strings.Contains(err.Error(), "not pending") {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("success=%d rejected=%d", success, rejected)
	}
}
