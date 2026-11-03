// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
	"testing"
)

func TestBuildConservative(t *testing.T) {
	m := &manifest.Manifest{Name: "demo", Services: []manifest.Service{{Name: "web", Image: "nginx:1.27"}, {Name: "db", Image: "postgres:17"}}}
	existing := []runtime.Container{{Names: []string{"orisema-demo-web"}, State: "running", Image: "nginx:1.27"}}
	steps := Build(m, existing)
	if len(steps) != 2 || steps[0].Action != Review || steps[1].Action != Create {
		t.Fatalf("unexpected steps: %#v", steps)
	}
}

func TestBuildBlocksUnimplementedSecretDelivery(t *testing.T) {
	m := &manifest.Manifest{Name: "demo", Services: []manifest.Service{{Name: "web", Image: "nginx:latest", Secrets: []manifest.SecretBinding{{Name: "token", Target: "token"}}}}}
	got := Build(m, nil)
	if len(got) != 1 || got[0].Action != Review || got[0].Reason != "secret delivery is declared but not implemented; no changes made" {
		t.Fatalf("Build() = %#v; want fail-closed REVIEW", got)
	}
}
