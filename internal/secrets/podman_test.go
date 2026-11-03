// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

func TestMain(m *testing.M) {
	if os.Getenv("ORISEMA_SECRET_ARGV_HELPER") == "1" {
		if len(os.Args) != 4 || os.Args[1] != "secret" || os.Args[2] != "inspect" {
			os.Exit(9)
		}
		if os.Getenv("ORISEMA_SECRET_INSPECT_FAIL") == "1" {
			os.Exit(1)
		}
		name := os.Args[len(os.Args)-1]
		if replacement := os.Getenv("ORISEMA_SECRET_RETURN_NAME"); replacement != "" {
			name = replacement
		}
		_ = json.NewEncoder(os.Stdout).Encode([]any{map[string]any{
			"ID": "abcdef0123456789abcdef012", "Spec": map[string]any{"Name": name},
		}})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestPodmanInspectorUsesMetadataOnlyArgv(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_SECRET_ARGV_HELPER", "1")
	got, err := (PodmanInspector{Binary: binary}).InspectMetadata(context.Background(), Reference{Provider: ProviderPodman, Locator: "database-password"})
	if err != nil {
		t.Fatal(err)
	}
	want := Metadata{Provider: ProviderPodman, Locator: "database-password", Identity: "abcdef0123456789abcdef012"}
	if got != want {
		t.Fatalf("InspectMetadata() = %#v; want %#v", got, want)
	}
}

func TestPodmanInspectorRejectsMismatchedMetadataWithoutCanary(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_SECRET_ARGV_HELPER", "1")
	t.Setenv("ORISEMA_SECRET_RETURN_NAME", "other")
	const canary = "canary-must-not-appear"
	_, err = (PodmanInspector{Binary: binary}).InspectMetadata(context.Background(), Reference{Provider: ProviderPodman, Locator: "database-password"})
	if err == nil || strings.Contains(err.Error(), canary) {
		t.Fatalf("InspectMetadata() error = %v; want safe mismatch", err)
	}
}

type recordingInspector struct {
	calls []Reference
}

func (r *recordingInspector) InspectMetadata(_ context.Context, ref Reference) (Metadata, error) {
	r.calls = append(r.calls, ref)
	return Metadata{Provider: ref.Provider, Locator: ref.Locator, Identity: "id-" + ref.Locator}, nil
}

func TestResolveManifestResolvesOnlyConsumedReferencesOnce(t *testing.T) {
	m := &manifest.Manifest{
		Secrets: []manifest.SecretDeclaration{
			{Name: "used", Source: manifest.SecretSource{Podman: &manifest.PodmanSecretSource{Name: "runtime-secret"}}},
			{Name: "unused", Source: manifest.SecretSource{Podman: &manifest.PodmanSecretSource{Name: "unused-secret"}}},
		},
		Services: []manifest.Service{
			{Name: "web", Secrets: []manifest.SecretBinding{{Name: "used", Target: "web-token"}}},
			{Name: "worker", Secrets: []manifest.SecretBinding{{Name: "used", Target: "worker-token"}}},
		},
	}
	inspector := &recordingInspector{}
	got, err := ResolveManifest(context.Background(), m, map[Provider]Inspector{ProviderPodman: inspector})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inspector.calls, []Reference{{Provider: ProviderPodman, Locator: "runtime-secret"}}) {
		t.Fatalf("calls = %#v", inspector.calls)
	}
	if got["web"][0].Identity != "id-runtime-secret" || got["worker"][0].Target != "worker-token" {
		t.Fatalf("resolved = %#v", got)
	}
}
