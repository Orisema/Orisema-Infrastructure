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

func TestHealthcheckValidation(t *testing.T) {
 base := Manifest{APIVersion:"orisema.dev/v1alpha1", Name:"demo", Services:[]Service{{Name:"web", Image:"nginx:1.27"}}}
 cases := []struct { name string; h Healthcheck; valid bool }{
  {"valid", Healthcheck{Command:"nginx -t",Interval:"30s",Timeout:"5s",Retries:3},true},
  {"missing command", Healthcheck{Interval:"30s",Timeout:"5s",Retries:3},false},
  {"short interval", Healthcheck{Command:"nginx -t",Interval:"500ms",Timeout:"5s",Retries:3},false},
  {"invalid timeout", Healthcheck{Command:"nginx -t",Interval:"30s",Timeout:"oops",Retries:3},false},
  {"zero retries", Healthcheck{Command:"nginx -t",Interval:"30s",Timeout:"5s"},false},
 }
 for _, tc := range cases { t.Run(tc.name,func(t *testing.T){
  m:=base
  m.Services=[]Service{{Name:"web",Image:"nginx:1.27",Healthcheck:&tc.h}}
  err:=m.Validate()
  if (err==nil)!=tc.valid {t.Fatalf("valid=%t err=%v",tc.valid,err)}
 })}
}
