// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"strings"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
	"github.com/Orisema/Orisema-Infrastructure/internal/secrets"
)

type fakeInspector struct {
	item runtime.Inspection
	err  error
}

func (f fakeInspector) Inspect(context.Context, string) (runtime.Inspection, error) {
	return f.item, f.err
}

func TestBuildInspected(t *testing.T) {
	m := &manifest.Manifest{Name: "hello-orisema", Services: []manifest.Service{{Name: "web", Image: "docker.io/library/nginx:1.27", Ports: []manifest.Port{{Host: 8080, Container: 80}}}}}
	existing := []runtime.Container{{Names: []string{"orisema-hello-orisema-web"}}}
	var item runtime.Inspection
	item.Config.Image = m.Services[0].Image
	item.Config.Labels = map[string]string{"dev.orisema.managed": "true", "dev.orisema.app": "hello-orisema", "dev.orisema.service": "web"}
	item.HostConfig.PortBindings = map[string][]runtime.Binding{"80/tcp": {{HostIP: "0.0.0.0", HostPort: "8080"}}}
	item.HostConfig.RestartPolicy.Name = "no"
	check := func(want Action) {
		t.Helper()
		got := BuildInspected(context.Background(), m, existing, fakeInspector{item: item})
		if len(got) != 1 || got[0].Action != want {
			t.Fatalf("want %s, got %#v", want, got)
		}
	}
	check(Noop)
	m.Services[0].Ports[0].HostIP = "127.0.0.1"
	item.HostConfig.PortBindings["80/tcp"][0].HostIP = "127.0.0.1"
	check(Noop)
	item.HostConfig.PortBindings["80/tcp"][0].HostIP = "0.0.0.0"
	check(Drift)
	m.Services[0].Ports[0].HostIP = "::1"
	item.HostConfig.PortBindings["80/tcp"][0].HostIP = "0:0:0:0:0:0:0:1"
	check(Noop)
	m.Services[0].Ports[0].HostIP = "127.0.0.1"
	item.HostConfig.PortBindings["80/tcp"][0].HostIP = "127.0.0.1"
	item.Config.Image = "docker.io/library/nginx:other"
	check(Drift)
	item.Config.Image = m.Services[0].Image
	delete(item.Config.Labels, "dev.orisema.managed")
	check(Review)
	existing = nil
	check(Create)
}

func TestBuildInspectedBlocksUnimplementedSecretDelivery(t *testing.T) {
	m := &manifest.Manifest{Name: "demo", Services: []manifest.Service{{Name: "web", Image: "nginx:latest", Secrets: []manifest.SecretBinding{{Name: "token", Target: "token"}}}}}
	got := BuildInspected(context.Background(), m, nil, fakeInspector{})
	if len(got) != 1 || got[0].Action != Review || got[0].Reason != "secret delivery is declared but not implemented; no changes made" {
		t.Fatalf("BuildInspected() = %#v; want fail-closed REVIEW", got)
	}
}

func TestBuildInspectedBlocksUnimplementedAlpha4Delivery(t *testing.T) {
	m := &manifest.Manifest{Name: "demo", Services: []manifest.Service{{
		Name: "web", Image: "nginx:latest",
		PublicEnvironment: []manifest.PublicEnvironmentEntry{{Name: "APP_MODE", Value: "laboratory"}},
		Networks:          []manifest.NetworkBinding{{Name: "private"}},
	}}}
	got := BuildInspected(context.Background(), m, nil, fakeInspector{})
	if len(got) != 1 || got[0].Action != Review || !strings.Contains(got[0].Reason, "not implemented") {
		t.Fatalf("BuildInspected() = %#v; want fail-closed REVIEW", got)
	}
	if strings.Contains(got[0].Reason, "laboratory") {
		t.Fatalf("plan reason exposed public value: %#v", got)
	}
}

func TestBuildInspectedResolvedComparesSecretIdentityLabels(t *testing.T) {
	m := &manifest.Manifest{Name: "demo", Services: []manifest.Service{{Name: "web", Image: "nginx:latest", Secrets: []manifest.SecretBinding{{Name: "token", Target: "token"}}}}}
	mounts := map[string][]secrets.ResolvedMount{"web": {{Name: "token", Provider: secrets.ProviderPodman, Identity: "secret-id", Target: "token"}}}
	got := BuildInspectedResolved(context.Background(), m, nil, fakeInspector{}, mounts)
	if len(got) != 1 || got[0].Action != Create {
		t.Fatalf("missing container plan = %#v; want CREATE", got)
	}
	item := runtime.Inspection{}
	item.Config.Image = "nginx:latest"
	item.Config.Labels = map[string]string{
		"dev.orisema.managed": "true", "dev.orisema.app": "demo", "dev.orisema.service": "web",
		"dev.orisema.secret.token": "podman:secret-id",
	}
	item.HostConfig.RestartPolicy.Name = "no"
	existing := []runtime.Container{{Names: []string{"orisema-demo-web"}}}
	got = BuildInspectedResolved(context.Background(), m, existing, fakeInspector{item: item}, mounts)
	if got[0].Action != Noop {
		t.Fatalf("matching secret plan = %#v; want NOOP", got)
	}
	item.Config.Labels["dev.orisema.secret.token"] = "podman:changed"
	got = BuildInspectedResolved(context.Background(), m, existing, fakeInspector{item: item}, mounts)
	if got[0].Action != Drift || !strings.Contains(got[0].Reason, "secret binding metadata differs") {
		t.Fatalf("changed secret plan = %#v; want DRIFT", got)
	}
}
