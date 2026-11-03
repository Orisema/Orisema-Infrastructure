// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"fmt"
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
			want := "field"
			if strings.HasPrefix(field, "secrets:") {
				want = "secrets are not part of orisema.dev/v1alpha2"
			}
			if _, err := Load(p); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("Load() error = %v; want rejection containing %q", err, want)
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
		{name: "v1alpha3 requires kind", mutate: func(m *Manifest) { m.APIVersion = "orisema.dev/v1alpha3" }, wantErr: "kind must be Application"},
		{name: "v1alpha3 preserves explicit loopback", mutate: func(m *Manifest) {
			m.APIVersion, m.Kind = "orisema.dev/v1alpha3", "Application"
			m.Services[0].Ports = []Port{{Host: 8080, Container: 80, HostIP: "127.0.0.1"}}
		}},
		{name: "v1alpha4 requires kind", mutate: func(m *Manifest) { m.APIVersion = "orisema.dev/v1alpha4" }, wantErr: "kind must be Application"},
		{name: "v1alpha4 preserves explicit loopback", mutate: func(m *Manifest) {
			m.APIVersion, m.Kind = "orisema.dev/v1alpha4", "Application"
			m.Services[0].Ports = []Port{{Host: 8080, Container: 80, HostIP: "127.0.0.1"}}
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

func TestV1Alpha4PublicConfigurationAndNetworks(t *testing.T) {
	valid := `apiVersion: orisema.dev/v1alpha4
kind: Application
name: demo
networks:
  - name: private
    source:
      podman:
        name: demo-private
services:
  - name: web
    image: nginx:latest
    publicEnvironment:
      - name: APP_MODE
        value: laboratory
      - name: EMPTY_VALUE
        value: ""
    networks:
      - name: private
`
	load := func(t *testing.T, content string) (*Manifest, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "manifest.yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return Load(path)
	}
	m, err := load(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Networks) != 1 || len(m.Services[0].Networks) != 1 || len(m.Services[0].PublicEnvironment) != 2 {
		t.Fatalf("unexpected decoded v1alpha4 manifest: %#v", m)
	}

	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{"v1alpha3 rejects networks", "orisema.dev/v1alpha4", "orisema.dev/v1alpha3", "networks are not part"},
		{"invalid environment name", "name: APP_MODE", "name: 1APP_MODE", "invalid publicEnvironment name"},
		{"duplicate environment name", "name: EMPTY_VALUE", "name: APP_MODE", "duplicate publicEnvironment name"},
		{"environment newline", "value: laboratory", "value: |\n          laboratory\n          second-line", "forbidden character"},
		{"missing network source", "source:\n      podman:\n        name: demo-private", "source: {}", "one valid podman name"},
		{"undeclared network", "name: private\n", "name: missing\n", "is not declared"},
		{"duplicate binding", "networks:\n      - name: private", "networks:\n      - name: private\n      - name: private", "bound more than once"},
		{"service without network", "    networks:\n      - name: private\n", "", "at least one declared network binding"},
		{"unused network", "services:\n", "  - name: unused\n    source:\n      podman:\n        name: demo-unused\nservices:\n", "declared but not consumed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, strings.Replace(valid, tc.old, tc.new, 1))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v; want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestOldContractsRejectAlpha4FieldPresence(t *testing.T) {
	for _, version := range []string{"orisema.dev/v1alpha1", "orisema.dev/v1alpha2", "orisema.dev/v1alpha3"} {
		for _, value := range []string{"[]", "null"} {
			for _, field := range []string{"top-networks", "service-networks", "public-environment"} {
				t.Run(version+"-"+field+"-"+value, func(t *testing.T) {
					kind := ""
					if version != "orisema.dev/v1alpha1" {
						kind = "kind: Application\n"
					}
					content := "apiVersion: " + version + "\n" + kind + "name: demo\n"
					if field == "top-networks" {
						content += "networks: " + value + "\n"
					}
					content += "services:\n  - name: web\n    image: nginx:latest\n"
					switch field {
					case "service-networks":
						content += "    networks: " + value + "\n"
					case "public-environment":
						content += "    publicEnvironment: " + value + "\n"
					}
					path := filepath.Join(t.TempDir(), "manifest.yaml")
					if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
						t.Fatal(err)
					}
					if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "not part of") {
						t.Fatalf("Load() error = %v; want version rejection", err)
					}
				})
			}
		}
	}
}

func TestV1Alpha4PublicEnvironmentLimitsAndNoValueReflection(t *testing.T) {
	entries := make([]PublicEnvironmentEntry, 129)
	for i := range entries {
		entries[i] = PublicEnvironmentEntry{Name: fmt.Sprintf("VALUE_%d", i), Value: "public"}
	}
	m := Manifest{APIVersion: "orisema.dev/v1alpha4", Kind: "Application", Name: "demo", Services: []Service{{Name: "web", Image: "nginx:latest", PublicEnvironment: entries}}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "at most 128 entries") {
		t.Fatalf("Validate() error = %v; want entry limit", err)
	}
	m.Services[0].PublicEnvironment = []PublicEnvironmentEntry{{Name: "PUBLIC_VALUE", Value: strings.Repeat("x", 4097)}}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "exceeds limits") {
		t.Fatalf("Validate() error = %v; want value limit", err)
	}
	if strings.Contains(err.Error(), strings.Repeat("x", 64)) {
		t.Fatalf("validation error reflected public value: %v", err)
	}
	total := make([]PublicEnvironmentEntry, 9)
	for i := range total {
		total[i] = PublicEnvironmentEntry{Name: fmt.Sprintf("VALUE_%d", i), Value: strings.Repeat("x", 4096)}
	}
	m.Services[0].PublicEnvironment = total
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "exceeds 32 KiB total") {
		t.Fatalf("Validate() error = %v; want total size limit", err)
	}

	networks := make([]NetworkDeclaration, 33)
	for i := range networks {
		name := fmt.Sprintf("network-%d", i)
		networks[i] = NetworkDeclaration{Name: name, Source: NetworkSource{Podman: &PodmanNetworkSource{Name: name}}}
	}
	m.Services[0].PublicEnvironment = nil
	m.Networks = networks
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "at most 32 declarations") {
		t.Fatalf("Validate() error = %v; want network declaration limit", err)
	}
	m.Networks = networks[:9]
	m.Services[0].Networks = make([]NetworkBinding, 9)
	for i := range m.Services[0].Networks {
		m.Services[0].Networks[i] = NetworkBinding{Name: networks[i].Name}
	}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "at most 8 bindings") {
		t.Fatalf("Validate() error = %v; want network binding limit", err)
	}
}

func TestV1Alpha3SecretReferences(t *testing.T) {
	valid := `apiVersion: orisema.dev/v1alpha3
kind: Application
name: demo
secrets:
  - name: database-password
    source:
      podman:
        name: demo-database-password
  - name: legacy-key
    source:
      file:
        path: /home/operator/.config/orisema/secrets/legacy-key
services:
  - name: web
    image: nginx:latest
    secrets:
      - name: database-password
        target: database-password
      - name: legacy-key
        target: legacy-key
`
	load := func(t *testing.T, content string) (*Manifest, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "manifest.yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return Load(path)
	}
	m, err := load(t, valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Secrets) != 2 || len(m.Services[0].Secrets) != 2 {
		t.Fatalf("unexpected decoded secrets: %#v", m)
	}

	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{"v1alpha2 rejects declarations", "orisema.dev/v1alpha3", "orisema.dev/v1alpha2", "secrets are not part"},
		{"missing source", "source:\n      podman:\n        name: demo-database-password", "source: {}", "exactly one"},
		{"multiple sources", "source:\n      podman:\n        name: demo-database-password", "source:\n      podman:\n        name: demo-database-password\n      file:\n        path: /safe/secret", "exactly one"},
		{"relative file", "/home/operator/.config/orisema/secrets/legacy-key", "relative/legacy-key", "clean and absolute"},
		{"unclean file", "/home/operator/.config/orisema/secrets/legacy-key", "/safe/../legacy-key", "clean and absolute"},
		{"undeclared binding", "name: legacy-key\n        target: legacy-key", "name: missing\n        target: missing", "is not declared"},
		{"invalid target", "target: legacy-key", "target: ../legacy-key", "valid file name"},
		{"duplicate target", "target: legacy-key", "target: database-password", "target \"database-password\" is used more than once"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, strings.Replace(valid, tc.old, tc.new, 1))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v; want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestOldContractsRejectSecretFieldPresence(t *testing.T) {
	for _, version := range []string{"orisema.dev/v1alpha1", "orisema.dev/v1alpha2"} {
		for _, value := range []string{"[]", "null"} {
			for _, level := range []string{"top", "service"} {
				t.Run(version+"-"+level+"-"+value, func(t *testing.T) {
					kind := ""
					if version == "orisema.dev/v1alpha2" {
						kind = "kind: Application\n"
					}
					content := "apiVersion: " + version + "\n" + kind + "name: demo\n"
					if level == "top" {
						content += "secrets: " + value + "\n"
					}
					content += "services:\n  - name: web\n    image: nginx:latest\n"
					if level == "service" {
						content += "    secrets: " + value + "\n"
					}
					path := filepath.Join(t.TempDir(), "manifest.yaml")
					if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
						t.Fatal(err)
					}
					if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "secrets are not part") {
						t.Fatalf("Load() error = %v; want version rejection", err)
					}
				})
			}
		}
	}
}

func TestV1Alpha3SecretValueIsNeverReflected(t *testing.T) {
	const canary = "alpha6-canary-do-not-disclose"
	content := "apiVersion: orisema.dev/v1alpha3\nkind: Application\nname: demo\nsecrets:\n  - name: token\n    source:\n      podman:\n        name: token\n        value: " + canary + "\nservices:\n  - name: web\n    image: nginx:latest\n"
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() unexpectedly accepted inline value")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("validation error exposed canary: %v", err)
	}
}

func TestV1Alpha3SecretLimits(t *testing.T) {
	declarations := make([]SecretDeclaration, 65)
	for i := range declarations {
		declarations[i] = SecretDeclaration{Name: fmt.Sprintf("secret-%d", i), Source: SecretSource{Podman: &PodmanSecretSource{Name: fmt.Sprintf("secret-%d", i)}}}
	}
	m := Manifest{APIVersion: "orisema.dev/v1alpha3", Kind: "Application", Name: "demo", Secrets: declarations, Services: []Service{{Name: "web", Image: "nginx:latest"}}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "at most 64 declarations") {
		t.Fatalf("Validate() error = %v; want declaration limit", err)
	}
	m.Secrets = declarations[:64]
	bindings := make([]SecretBinding, 65)
	for i := range bindings {
		bindings[i] = SecretBinding{Name: fmt.Sprintf("secret-%d", i%64), Target: fmt.Sprintf("target-%d", i)}
	}
	m.Services[0].Secrets = bindings
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "at most 64 bindings") {
		t.Fatalf("Validate() error = %v; want binding limit", err)
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
