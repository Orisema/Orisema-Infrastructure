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

func configuredIntentRequest(id string, at time.Time) UpdateIntentRequest {
	request := intentRequest(id, at)
	request.Bindings = &UpdateBindingEvidence{
		PublicEnvironmentNames:  []string{"HTTP_PORT", "APP_MODE"},
		PublicEnvironmentDigest: strings.Repeat("a", 64),
		Secrets: []UpdateSecretEvidence{
			{Name: "token", Provider: "file", Target: "token", Identity: "file-identity"},
			{Name: "database", Provider: "podman", Target: "database", Identity: "podman-identity"},
		},
		Networks: []UpdateNetworkEvidence{{Name: "private", Provider: "podman", Identity: strings.Repeat("b", 64), Driver: "bridge"}},
	}
	return request
}

func TestCreateUpdateIntentV2PersistsCanonicalEvidenceWithoutValuesOrLocators(t *testing.T) {
	store, _, now := fixture(t)
	got, err := store.CreateUpdateIntent(configuredIntentRequest("intent-v2", now))
	if err != nil {
		t.Fatal(err)
	}
	if got.Intent.Contract != updateIntentV2 || got.Intent.ConfigurationFingerprint == "" || got.Intent.Bindings == nil {
		t.Fatalf("intent=%#v", got.Intent)
	}
	if strings.Join(got.Intent.Bindings.PublicEnvironmentNames, ",") != "APP_MODE,HTTP_PORT" || got.Intent.Bindings.Secrets[0].Name != "database" {
		t.Fatalf("binding evidence is not canonical: %#v", got.Intent.Bindings)
	}
	data, err := os.ReadFile(filepath.Join(store.root, got.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"public-value", "/secret/path", "locator"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("journal contains forbidden %q: %s", forbidden, data)
		}
	}
	reopened, err := OpenExistingStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Load(got.ID); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateIntentV2FingerprintIsOrderIndependentAndIdentitySensitive(t *testing.T) {
	storeA, _, now := fixture(t)
	requestA := configuredIntentRequest("intent-order-a", now)
	gotA, err := storeA.CreateUpdateIntent(requestA)
	if err != nil {
		t.Fatal(err)
	}

	storeB, _, _ := fixture(t)
	requestB := configuredIntentRequest("intent-order-b", now)
	requestB.Bindings.PublicEnvironmentNames[0], requestB.Bindings.PublicEnvironmentNames[1] = requestB.Bindings.PublicEnvironmentNames[1], requestB.Bindings.PublicEnvironmentNames[0]
	requestB.Bindings.Secrets[0], requestB.Bindings.Secrets[1] = requestB.Bindings.Secrets[1], requestB.Bindings.Secrets[0]
	gotB, err := storeB.CreateUpdateIntent(requestB)
	if err != nil {
		t.Fatal(err)
	}
	if gotA.Intent.PlanFingerprint != gotB.Intent.PlanFingerprint {
		t.Fatal("equivalent binding order changed the fingerprint")
	}

	storeC, _, _ := fixture(t)
	requestC := configuredIntentRequest("intent-identity", now)
	requestC.Bindings.Secrets[0].Identity = "changed-identity"
	gotC, err := storeC.CreateUpdateIntent(requestC)
	if err != nil {
		t.Fatal(err)
	}
	if gotA.Intent.PlanFingerprint == gotC.Intent.PlanFingerprint {
		t.Fatal("changed secret identity did not change the fingerprint")
	}
}

func TestCreateUpdateIntentV2RejectsInvalidEvidence(t *testing.T) {
	store, _, now := fixture(t)
	request := configuredIntentRequest("intent-v2-invalid", now)
	request.Bindings.Secrets = append(request.Bindings.Secrets, request.Bindings.Secrets[0])
	if _, err := store.CreateUpdateIntent(request); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
}

func TestPublicConfigurationIntentPersistsValueFreeAndCannotAuthorize(t *testing.T) {
	store, _, now := fixture(t)
	request := intentRequest("public-config-intent", now)
	request.TargetImage = request.CurrentImage
	request.Bindings = &UpdateBindingEvidence{
		PublicEnvironmentNames:  []string{"APP_MODE"},
		PublicEnvironmentDigest: strings.Repeat("a", 64),
	}
	created, err := store.CreatePublicConfigurationIntent(request, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if created.State != Pending || created.Intent.Contract != updateIntentV3PublicConfig || created.Intent.DifferenceFingerprint == "" {
		t.Fatalf("operation=%#v", created)
	}
	path := filepath.Join(store.root, created.ID+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before), "public-value-canary") {
		t.Fatalf("journal exposed value: %s", before)
	}
	if _, err := store.AuthorizeUpdateIntent(created.ID, created.Intent.PlanFingerprint, observedIntentResource(), now.Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "review-only") {
		t.Fatalf("authorization error=%v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("rejected authorization changed public configuration journal")
	}
	authorized, err := store.AuthorizePublicConfigurationIntent(created.ID, created.Intent.PlanFingerprint, observedIntentResource(), now.Add(time.Minute))
	if err != nil || authorized.State != Preparing || authorized.Authorization == nil {
		t.Fatalf("authorized=%#v err=%v", authorized, err)
	}
	executing, err := store.BeginUpdateExecution(created.ID, created.Intent.PlanFingerprint, "rollback-slot", observedIntentResource(), now.Add(2*time.Minute))
	if err != nil || executing.State != Applying || executing.Execution == nil || executing.Execution.PendingStep != "stop-previous" {
		t.Fatalf("executing=%#v err=%v", executing, err)
	}
}

func TestConcurrentPublicConfigurationAuthorizationHasOneWinner(t *testing.T) {
	store, _, now := fixture(t)
	request := intentRequest("public-config-concurrent", now)
	request.TargetImage = request.CurrentImage
	request.Bindings = &UpdateBindingEvidence{PublicEnvironmentNames: []string{"APP_MODE"}, PublicEnvironmentDigest: strings.Repeat("a", 64)}
	created, err := store.CreatePublicConfigurationIntent(request, strings.Repeat("b", 64))
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
			_, err := store.AuthorizePublicConfigurationIntent(created.ID, created.Intent.PlanFingerprint, observedIntentResource(), now.Add(time.Minute))
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

func TestPublicConfigurationAuthorizationFailuresDoNotWrite(t *testing.T) {
	tests := []struct {
		name        string
		fingerprint func(Operation) string
		offset      time.Duration
		want        string
	}{
		{"changed plan", func(Operation) string { return strings.Repeat("0", 64) }, time.Minute, "fingerprint"},
		{"expired", func(op Operation) string { return op.Intent.PlanFingerprint }, UpdateAuthorizationTTL + time.Second, "expired"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, _, now := fixture(t)
			request := intentRequest("public-config-failure", now)
			request.TargetImage = request.CurrentImage
			request.Bindings = &UpdateBindingEvidence{PublicEnvironmentNames: []string{"APP_MODE"}, PublicEnvironmentDigest: strings.Repeat("a", 64)}
			created, err := store.CreatePublicConfigurationIntent(request, strings.Repeat("b", 64))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(store.root, created.ID+".json")
			before, _ := os.ReadFile(path)
			_, err = store.AuthorizePublicConfigurationIntent(created.ID, tc.fingerprint(created), observedIntentResource(), now.Add(tc.offset))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v; want %q", err, tc.want)
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("failed public configuration authorization changed journal")
			}
		})
	}
}

func TestPublicConfigurationIntentRejectsOtherBindings(t *testing.T) {
	store, _, now := fixture(t)
	request := intentRequest("public-config-invalid", now)
	request.TargetImage = request.CurrentImage
	request.Bindings = &UpdateBindingEvidence{PublicEnvironmentNames: []string{"APP_MODE"}, PublicEnvironmentDigest: strings.Repeat("a", 64), Secrets: []UpdateSecretEvidence{{Name: "token", Provider: "podman", Target: "token", Identity: "id"}}}
	if _, err := store.CreatePublicConfigurationIntent(request, strings.Repeat("b", 64)); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRejectsAdvancedPublicConfigurationIntent(t *testing.T) {
	store, _, now := fixture(t)
	request := intentRequest("public-config-advanced", now)
	request.TargetImage = request.CurrentImage
	request.Bindings = &UpdateBindingEvidence{PublicEnvironmentNames: []string{"APP_MODE"}, PublicEnvironmentDigest: strings.Repeat("a", 64)}
	created, err := store.CreatePublicConfigurationIntent(request, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, created.ID+".json")
	data, _ := os.ReadFile(path)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["state"] = string(Preparing)
	data, _ = json.Marshal(raw)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(created.ID); err == nil {
		t.Fatalf("load error=%v", err)
	}
}

func TestLoadRejectsTamperedUpdateIntentV2Bindings(t *testing.T) {
	store, _, now := fixture(t)
	got, err := store.CreateUpdateIntent(configuredIntentRequest("intent-v2-tamper", now))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, got.ID+".json")
	data, _ := os.ReadFile(path)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	bindings := raw["intent"].(map[string]any)["bindings"].(map[string]any)
	bindings["networks"].([]any)[0].(map[string]any)["identity"] = strings.Repeat("c", 64)
	data, _ = json.Marshal(raw)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(got.ID); err == nil || !strings.Contains(err.Error(), "binding evidence") {
		t.Fatalf("expected binding tampering rejection, got %v", err)
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
