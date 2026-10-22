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
		Name: "demo",
		Services: []Service{{Name: "web", Image: "docker.io/library/nginx:1.27", Ports: []Port{{Host: 8080, Container: 80}}}},
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
	p := filepath.Join(t.TempDir(), "bad.yaml")
	content := "apiVersion: orisema.dev/v1alpha1\nname: demo\nservices:\n  - name: web\n    image: nginx:latest\n    priviliged: true\n"
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected unknown field error")
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
