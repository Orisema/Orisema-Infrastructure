// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

func TestRestartPolicyComparison(t *testing.T) {
	for _, tc := range []struct {
		name          string
		declared      *manifest.Restart
		actualName    string
		actualRetries int
		want          Action
	}{
		{"omitted matches runtime default", nil, "no", 0, Noop},
		{"explicit no", &manifest.Restart{Policy: "no"}, "no", 0, Noop},
		{"bounded on failure", &manifest.Restart{Policy: "on-failure", MaxRetries: 3}, "on-failure", 3, Noop},
		{"different policy", &manifest.Restart{Policy: "on-failure", MaxRetries: 3}, "no", 0, Drift},
		{"different retry limit", &manifest.Restart{Policy: "on-failure", MaxRetries: 3}, "on-failure", 5, Drift},
		{"unexpected always", nil, "always", 0, Drift},
		{"empty inspection is not equivalence", nil, "", 0, Drift},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &manifest.Manifest{Name: "restart-lab", Services: []manifest.Service{{Name: "web", Image: "nginx:stable", Restart: tc.declared}}}
			var item runtime.Inspection
			item.Config.Image = "nginx:stable"
			item.Config.Labels = map[string]string{"dev.orisema.managed": "true", "dev.orisema.app": "restart-lab", "dev.orisema.service": "web"}
			item.HostConfig.RestartPolicy.Name = tc.actualName
			item.HostConfig.RestartPolicy.MaximumRetryCount = tc.actualRetries
			existing := []runtime.Container{{Names: []string{"orisema-restart-lab-web"}}}
			got := BuildInspected(context.Background(), m, existing, fakeInspector{item: item})
			if len(got) != 1 || got[0].Action != tc.want {
				t.Fatalf("got %#v, want %s", got, tc.want)
			}
		})
	}
}
