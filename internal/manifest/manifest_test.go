// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	good := &Manifest{
		APIVersion: "orisema.dev/v1alpha1",
		Name:       "demo",
		Services:   []Service{{Name: "web", Image: "docker.io/library/nginx:1.27", Ports: []Port{{Host: 8080, Container: 80}}}},
	}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := *good
	bad.Services = append([]Service{}, good.Services...)
	bad.Services = append(bad.Services, good.Services[0])
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate service") {
		t.Fatalf("expected duplicate service error, got %v", err)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	for _, field := range []string{"environment: []", "volumes: []", "secrets: []", "securityContext: {}", "privileged: true"} {
		t.Run(strings.Fields(field)[0], func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "bad.yaml")
			content := "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:latest\n    " + field + "\n"
			if err := os.WriteFile(p, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "field") {
				t.Fatalf("Load() error = %v; want unknown field rejection", err)
			}
		})
	}
}

func TestUnknownSecretValueIsNotReflectedInError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secret.yaml")
	const secret = "do-not-echo-this-value"
	content := "apiVersion: orisema.dev/v1alpha2\nkind: Application\nname: demo\nservices:\n  - name: web\n    image: nginx:latest\n    secrets:\n      TOKEN: " + secret + "\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil {
		t.Fatal("Load() unexpectedly accepted secrets")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error exposed rejected secret value: %v", err)
	}
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	content := "apiVersion: orisema.dev/v1alpha1\nname: demo\nservices:\n  - name: web\n    image: nginx:latest\n---\nname: other\n"
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected multiple document error")
	}
}

func TestManifestVersionContracts(t *testing.T) {
	base := Manifest{
		APIVersion: "orisema.dev/v1alpha1",
		Name:       "demo",
		Services:   []Service{{Name: "web", Image: "nginx:1.27"}},
	}
	tests := []struct {
		name    string
		mutate  func(*Manifest)
		wantErr string
	}{
		{name: "v1alpha1 remains valid"},
		{name: "v1alpha1 rejects kind", mutate: func(m *Manifest) { m.Kind = "Application" }, wantErr: "kind is not part"},
		{name: "v1alpha1 rejects hostIP", mutate: func(m *Manifest) {
			m.Services[0].Ports = []Port{{Host: 8080, Container: 80, HostIP: "127.0.0.1"}}
		}, wantErr: "hostIP is not part"},
		{name: "v1alpha2 requires kind", mutate: func(m *Manifest) { m.APIVersion = "orisema.dev/v1alpha2" }, wantErr: "kind must be Application"},
		{name: "v1alpha2 requires explicit hostIP", mutate: func(m *Manifest) {
			m.APIVersion, m.Kind = "orisema.dev/v1alpha2", "Application"
			m.Services[0].Ports = []Port{{Host: 8080, Container: 80}}
		}, wantErr: "explicit loopback IP"},
		{name: "v1alpha2 rejects public bind", mutate: func(m *Manifest) {
			m.APIVersion, m.Kind = "orisema.dev/v1alpha2", "Application"
			m.Services[0].Ports = []Port{{Host: 8080, Container: 80, HostIP: "0.0.0.0"}}
		}, wantErr: "explicit loopback IP"},
		{name: "v1alpha2 rejects duplicate container port", mutate: func(m *Manifest) {
			m.APIVersion, m.Kind = "orisema.dev/v1alpha2", "Application"
			m.Services[0].Ports = []Port{
				{Host: 8080, Container: 80, HostIP: "127.0.0.1"},
				{Host: 8081, Container: 80, HostIP: "127.0.0.1"},
			}
		}, wantErr: "container port 80 is published more than once"},
		{name: "v1alpha2 accepts IPv4 loopback", mutate: func(m *Manifest) {
			m.APIVersion, m.Kind = "orisema.dev/v1alpha2", "Application"
			m.Services[0].Ports = []Port{{Host: 8080, Container: 80, HostIP: "127.0.0.1"}}
		}},
		{name: "v1alpha2 accepts IPv6 loopback", mutate: func(m *Manifest) {
			m.APIVersion, m.Kind = "orisema.dev/v1alpha2", "Application"
			m.Services[0].Ports = []Port{{Host: 8080, Container: 80, HostIP: "::1"}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			m.Services = append([]Service(nil), base.Services...)
			if tc.mutate != nil {
				tc.mutate(&m)
			}
			err := m.Validate()
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v; want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestHealthcheckValidation(t *testing.T) {
	base := Manifest{APIVersion: "orisema.dev/v1alpha1", Name: "demo", Services: []Service{{Name: "web", Image: "nginx:1.27"}}}
	cases := []struct {
		name  string
		h     Healthcheck
		valid bool
	}{
		{"valid", Healthcheck{Command: "nginx -t", Interval: "30s", Timeout: "5s", Retries: 3}, true},
		{"missing command", Healthcheck{Interval: "30s", Timeout: "5s", Retries: 3}, false},
		{"short interval", Healthcheck{Command: "nginx -t", Interval: "500ms", Timeout: "5s", Retries: 3}, false},
		{"invalid timeout", Healthcheck{Command: "nginx -t", Interval: "30s", Timeout: "oops", Retries: 3}, false},
		{"zero retries", Healthcheck{Command: "nginx -t", Interval: "30s", Timeout: "5s"}, false},
		{"newline command", Healthcheck{Command: "true\nfalse", Interval: "30s", Timeout: "5s", Retries: 3}, false},
		{"NUL command", Healthcheck{Command: "true\x00", Interval: "30s", Timeout: "5s", Retries: 3}, false},
		{"overflow duration", Healthcheck{Command: "true", Interval: "999999999999h", Timeout: "5s", Retries: 3}, false},
		{"negative timeout", Healthcheck{Command: "true", Interval: "30s", Timeout: "-1s", Retries: 3}, false},
		{"too many retries", Healthcheck{Command: "true", Interval: "30s", Timeout: "5s", Retries: 101}, false},
		{"minimum values", Healthcheck{Command: "true", Interval: "1s", Timeout: "1s", Retries: 1}, true},
		{"maximum retries", Healthcheck{Command: "true", Interval: "30s", Timeout: "5s", Retries: 100}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			m.Services = []Service{{Name: "web", Image: "nginx:1.27", Healthcheck: &tc.h}}
			err := m.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t err=%v", tc.valid, err)
			}
		})
	}
}
