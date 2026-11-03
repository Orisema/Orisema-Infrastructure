// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package networks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

const testNetworkID = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func TestMain(m *testing.M) {
	if os.Getenv("ORISEMA_NETWORK_ARGV_HELPER") == "1" {
		if len(os.Args) != 4 || os.Args[1] != "network" || os.Args[2] != "inspect" {
			os.Exit(9)
		}
		if os.Getenv("ORISEMA_NETWORK_INSPECT_FAIL") == "1" {
			os.Exit(1)
		}
		if os.Getenv("ORISEMA_NETWORK_OVERSIZE") == "1" {
			_, _ = os.Stdout.WriteString(strings.Repeat("x", maxPodmanNetworkMetadata+1))
			os.Exit(0)
		}
		name := os.Args[3]
		if replacement := os.Getenv("ORISEMA_NETWORK_RETURN_NAME"); replacement != "" {
			name = replacement
		}
		id := testNetworkID
		if replacement := os.Getenv("ORISEMA_NETWORK_RETURN_ID"); replacement != "" {
			id = replacement
		}
		driver := "bridge"
		if replacement := os.Getenv("ORISEMA_NETWORK_RETURN_DRIVER"); replacement != "" {
			driver = replacement
		}
		records := []podmanNetworkRecord{{Name: name, ID: id, Driver: driver}}
		if os.Getenv("ORISEMA_NETWORK_RETURN_AMBIGUOUS") == "1" {
			records = append(records, records[0])
		}
		_ = json.NewEncoder(os.Stdout).Encode(records)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestPodmanInspectorUsesMetadataOnlyArgv(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_NETWORK_ARGV_HELPER", "1")
	got, err := (PodmanInspector{Binary: binary}).InspectMetadata(context.Background(), Reference{Provider: ProviderPodman, Locator: "demo-private"})
	if err != nil {
		t.Fatal(err)
	}
	want := Metadata{Provider: ProviderPodman, Locator: "demo-private", Identity: testNetworkID, Driver: "bridge"}
	if got != want {
		t.Fatalf("InspectMetadata() = %#v; want %#v", got, want)
	}
}

func TestPodmanInspectorRejectsUnsafeMetadata(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		ref  Reference
		env  map[string]string
		want string
	}{
		{name: "invalid provider", ref: Reference{Provider: "other", Locator: "demo-private"}, want: "invalid Podman network reference"},
		{name: "option-like locator", ref: Reference{Provider: ProviderPodman, Locator: "--format"}, want: "invalid Podman network reference"},
		{name: "mismatched name", ref: Reference{Provider: ProviderPodman, Locator: "demo-private"}, env: map[string]string{"ORISEMA_NETWORK_RETURN_NAME": "other"}, want: "mismatched network metadata"},
		{name: "invalid identity", ref: Reference{Provider: ProviderPodman, Locator: "demo-private"}, env: map[string]string{"ORISEMA_NETWORK_RETURN_ID": "short"}, want: "invalid network metadata"},
		{name: "ambiguous metadata", ref: Reference{Provider: ProviderPodman, Locator: "demo-private"}, env: map[string]string{"ORISEMA_NETWORK_RETURN_AMBIGUOUS": "1"}, want: "invalid network metadata"},
		{name: "unsupported driver", ref: Reference{Provider: ProviderPodman, Locator: "demo-private"}, env: map[string]string{"ORISEMA_NETWORK_RETURN_DRIVER": "macvlan"}, want: "unsupported; expected bridge"},
		{name: "bounded output", ref: Reference{Provider: ProviderPodman, Locator: "demo-private"}, env: map[string]string{"ORISEMA_NETWORK_OVERSIZE": "1"}, want: "exceeds 64 KiB limit"},
		{name: "runtime failure", ref: Reference{Provider: ProviderPodman, Locator: "demo-private"}, env: map[string]string{"ORISEMA_NETWORK_INSPECT_FAIL": "1"}, want: "metadata unavailable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ORISEMA_NETWORK_ARGV_HELPER", "1")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			_, err := (PodmanInspector{Binary: binary}).InspectMetadata(context.Background(), tc.ref)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("InspectMetadata() error = %v; want %q", err, tc.want)
			}
		})
	}
}

type recordingInspector struct {
	calls    []Reference
	identity string
}

func (r *recordingInspector) InspectMetadata(_ context.Context, ref Reference) (Metadata, error) {
	r.calls = append(r.calls, ref)
	return Metadata{Provider: ref.Provider, Locator: ref.Locator, Identity: r.identity, Driver: "bridge"}, nil
}

func TestResolveManifestResolvesOnlyConsumedReferencesOnce(t *testing.T) {
	m := &manifest.Manifest{
		Networks: []manifest.NetworkDeclaration{
			{Name: "used", Source: manifest.NetworkSource{Podman: &manifest.PodmanNetworkSource{Name: "runtime-network"}}},
			{Name: "unused", Source: manifest.NetworkSource{Podman: &manifest.PodmanNetworkSource{Name: "unused-network"}}},
		},
		Services: []manifest.Service{
			{Name: "web", Networks: []manifest.NetworkBinding{{Name: "used"}}},
			{Name: "worker", Networks: []manifest.NetworkBinding{{Name: "used"}}},
		},
	}
	inspector := &recordingInspector{identity: testNetworkID}
	got, err := ResolveManifest(context.Background(), m, map[Provider]Inspector{ProviderPodman: inspector})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inspector.calls, []Reference{{Provider: ProviderPodman, Locator: "runtime-network"}}) {
		t.Fatalf("calls = %#v", inspector.calls)
	}
	if got["web"][0].Identity != testNetworkID || got["worker"][0].Name != "used" {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestRevalidateManifestRejectsIdentityChange(t *testing.T) {
	m := &manifest.Manifest{
		Networks: []manifest.NetworkDeclaration{{Name: "private", Source: manifest.NetworkSource{Podman: &manifest.PodmanNetworkSource{Name: "runtime-network"}}}},
		Services: []manifest.Service{{Name: "web", Networks: []manifest.NetworkBinding{{Name: "private"}}}},
	}
	inspector := &recordingInspector{identity: testNetworkID}
	providers := map[Provider]Inspector{ProviderPodman: inspector}
	expected, err := ResolveManifest(context.Background(), m, providers)
	if err != nil {
		t.Fatal(err)
	}
	if err := RevalidateManifest(context.Background(), m, providers, expected); err != nil {
		t.Fatalf("matching revalidation failed: %v", err)
	}
	inspector.identity = strings.Repeat("1", 64)
	err = RevalidateManifest(context.Background(), m, providers, expected)
	if err == nil || !strings.Contains(err.Error(), "identity changed; no changes made") {
		t.Fatalf("RevalidateManifest() error = %v; want identity rejection", err)
	}
}

func TestSameBindings(t *testing.T) {
	base := []ResolvedBinding{{Name: "private", Provider: ProviderPodman, Locator: "runtime-network", Identity: testNetworkID, Driver: "bridge"}}
	copyOfBase := append([]ResolvedBinding(nil), base...)
	if !SameBindings(base, copyOfBase) {
		t.Fatal("equal bindings differ")
	}
	copyOfBase[0].Identity = strings.Repeat("1", 64)
	if SameBindings(base, copyOfBase) {
		t.Fatal("identity change was accepted")
	}
}

func TestPodmanNetworkIntegrationReadOnly(t *testing.T) {
	if os.Getenv("ORISEMA_PODMAN_NETWORK_INTEGRATION") != "1" {
		t.Skip("set ORISEMA_PODMAN_NETWORK_INTEGRATION=1 to run")
	}
	name := fmt.Sprintf("orisema-alpha7-network-%d", os.Getpid())
	if err := createIntegrationNetwork(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := removeIntegrationNetwork(name); err != nil {
			t.Errorf("cleanup network: %v", err)
		}
	})
	first, err := (PodmanInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderPodman, Locator: name})
	if err != nil {
		t.Fatal(err)
	}
	second, err := (PodmanInspector{}).InspectMetadata(context.Background(), Reference{Provider: ProviderPodman, Locator: name})
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.Driver != "bridge" || len(first.Identity) != 64 {
		t.Fatalf("unstable metadata: first=%#v second=%#v", first, second)
	}
	if err := integrationNetworkExists(name); err != nil {
		t.Fatalf("read-only inspection did not preserve external network: %v", err)
	}
}
