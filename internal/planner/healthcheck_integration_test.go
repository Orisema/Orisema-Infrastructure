// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"os"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

// Opt-in, read-only verification of the isolated lab after deployment.
func TestPodmanHealthcheckIntegrationReadOnly(t *testing.T) {
	path := os.Getenv("ORISEMA_TEST_HEALTH_MANIFEST")
	if path == "" {
		t.Skip("set ORISEMA_TEST_HEALTH_MANIFEST to a deployed healthcheck manifest")
	}
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range m.Services {
		if svc.Healthcheck == nil {
			t.Fatalf("service %s must declare a healthcheck", svc.Name)
		}
	}
	client := runtime.Client{}
	existing, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range BuildInspected(context.Background(), m, existing, client) {
		if step.Action != Noop {
			t.Errorf("%s: got %s: %s", step.Name, step.Action, step.Reason)
		}
	}
	// Verify drift against real inspect data without changing the container.
	for i := range m.Services {
		m.Services[i].Healthcheck.Command += " && true"
	}
	for _, step := range BuildInspected(context.Background(), m, existing, client) {
		if step.Action != Drift {
			t.Errorf("%s: modified command must be DRIFT; got %s: %s", step.Name, step.Action, step.Reason)
		}
	}
}
