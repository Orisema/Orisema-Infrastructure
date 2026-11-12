// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package updateplan

import (
	"strings"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/planner"
)

func updateManifest() *manifest.Manifest {
	return &manifest.Manifest{Name: "demo", Services: []manifest.Service{{
		Name: "web", Image: "nginx:new",
		Healthcheck: &manifest.Healthcheck{Command: "nginx -t", Interval: "30s", Timeout: "5s", Retries: 3},
	}}}
}

func TestBuildAllowsOnlyImageOnlyDriftWithHealthcheck(t *testing.T) {
	m := updateManifest()
	evidence := []planner.Step{{Name: "orisema-demo-web", Image: "nginx:new", CurrentImage: "nginx:old", CurrentID: "container-123", Action: planner.Drift, Differences: []string{"configured image differs"}, TypedDifferences: []planner.Difference{{Kind: planner.DifferenceImage, Detail: "configured image differs"}}, Reason: "configured image differs"}}
	got := Build(m, evidence)
	if len(got) != 1 || got[0].Action != Candidate || got[0].CurrentImage != "nginx:old" {
		t.Fatalf("plan=%#v", got)
	}
	if len(got[0].DifferenceKinds) != 1 || got[0].DifferenceKinds[0] != planner.DifferenceImage || len(got[0].DifferenceFingerprint) != 64 {
		t.Fatalf("typed evidence=%#v", got[0])
	}
	m.Services[0].Healthcheck = nil
	got = Build(m, evidence)
	if got[0].Action != Blocked || !strings.Contains(got[0].Reason, "healthcheck") {
		t.Fatalf("plan=%#v", got)
	}
}

func TestBuildBlocksMissingReviewAndCompoundDrift(t *testing.T) {
	m := updateManifest()
	tests := []struct {
		name string
		step planner.Step
		want string
	}{
		{"missing", planner.Step{Name: "orisema-demo-web", Action: planner.Create}, "deploy"},
		{"review", planner.Step{Name: "orisema-demo-web", Action: planner.Review, Reason: "ownership labels missing"}, "ownership"},
		{"compound", planner.Step{Name: "orisema-demo-web", CurrentImage: "nginx:old", CurrentID: "container-123", Action: planner.Drift, Differences: []string{"configured image differs", "published port binding differs"}, Reason: "configured image differs, published port binding differs"}, "image-only"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Build(m, []planner.Step{tc.step})
			if got[0].Action != Blocked || !strings.Contains(got[0].Reason, tc.want) {
				t.Fatalf("plan=%#v", got)
			}
		})
	}
}

func TestBuildRejectsLegacyOrUnknownDifferenceEvidence(t *testing.T) {
	m := updateManifest()
	for _, evidence := range []planner.Step{
		{Name: "orisema-demo-web", CurrentImage: "nginx:old", CurrentID: "container-123", Action: planner.Drift, Differences: []string{"configured image differs"}, Reason: "configured image differs"},
		{Name: "orisema-demo-web", CurrentImage: "nginx:old", CurrentID: "container-123", Action: planner.Drift, TypedDifferences: []planner.Difference{{Kind: planner.DifferenceUnknown, Detail: "future field differs"}}, Reason: "future field differs"},
		{Name: "orisema-demo-web", CurrentImage: "nginx:old", CurrentID: "container-123", Action: planner.Drift, TypedDifferences: []planner.Difference{{Kind: planner.DifferenceSecretBinding, Detail: "secret binding metadata differs"}}, Reason: "secret binding metadata differs"},
		{Name: "orisema-demo-web", CurrentImage: "nginx:old", CurrentID: "container-123", Action: planner.Drift, TypedDifferences: []planner.Difference{{Kind: planner.DifferenceNetworkBinding, Detail: "network attachment differs"}}, Reason: "network attachment differs"},
	} {
		got := Build(m, []planner.Step{evidence})
		if got[0].Action != Blocked || !strings.Contains(got[0].Reason, "image-only") {
			t.Fatalf("plan=%#v; want fail-closed BLOCKED", got)
		}
	}
}

func TestBuildAllowsPublicConfigurationPrepareOnly(t *testing.T) {
	m := updateManifest()
	evidence := planner.Step{Name: "orisema-demo-web", CurrentImage: "nginx:new", CurrentID: "container-123", Action: planner.Drift, TypedDifferences: []planner.Difference{{Kind: planner.DifferencePublicConfig, Detail: "public configuration differs"}}, Reason: "public configuration differs"}
	got := Build(m, []planner.Step{evidence})
	if got[0].Action != PublicConfigPrepare || got[0].CurrentImage != got[0].TargetImage {
		t.Fatalf("plan=%#v; want prepare-only public configuration candidate", got)
	}
	m.Services[0].Secrets = []manifest.SecretBinding{{Name: "token", Target: "token"}}
	got = Build(m, []planner.Step{evidence})
	if got[0].Action != Blocked || !strings.Contains(got[0].Reason, "without secret or network") {
		t.Fatalf("plan=%#v; want configured binding block", got)
	}
}

func TestBuildNoopAndMissingEvidence(t *testing.T) {
	m := updateManifest()
	got := Build(m, []planner.Step{{Name: "orisema-demo-web", Action: planner.Noop}})
	if got[0].Action != Noop {
		t.Fatalf("plan=%#v", got)
	}
	got = Build(m, nil)
	if got[0].Action != Blocked || !strings.Contains(got[0].Reason, "missing") {
		t.Fatalf("plan=%#v", got)
	}
}
