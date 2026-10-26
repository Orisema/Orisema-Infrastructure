// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package diagnostic

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

type fakeRuntime struct {
	containers []runtime.Container
	items      map[string]runtime.Inspection
	listErr    error
	inspectErr error
}

func (f fakeRuntime) List(context.Context) ([]runtime.Container, error) {
	return f.containers, f.listErr
}

func (f fakeRuntime) Inspect(_ context.Context, name string) (runtime.Inspection, error) {
	return f.items[name], f.inspectErr
}

func TestBuildOperationalConditions(t *testing.T) {
	manifest := testManifest()
	for _, tc := range []struct {
		name        string
		client      fakeRuntime
		wantConfig  string
		wantRun     string
		wantHealth  string
		wantFailure string
		wantAction  string
		wantTyped   Action
	}{
		{
			name:       "missing",
			client:     fakeRuntime{},
			wantConfig: "absent", wantRun: "not found", wantHealth: "unavailable",
			wantFailure: "does not exist", wantAction: "deploy with explicit approval", wantTyped: ActionRedeployReview,
		},
		{
			name:       "running healthy",
			client:     managedRuntime(inspection("running", true, "healthy", 0)),
			wantConfig: "matches declared fields", wantRun: "running", wantHealth: "healthy",
			wantFailure: "none detected", wantAction: "none", wantTyped: ActionNone,
		},
		{
			name:       "running unhealthy",
			client:     managedRuntime(inspection("running", true, "unhealthy", 0)),
			wantConfig: "matches declared fields", wantRun: "running", wantHealth: "unhealthy",
			wantFailure: "healthcheck is unhealthy", wantAction: "investigate healthcheck", wantTyped: ActionInvestigate,
		},
		{
			name:       "stopped ignores stale health",
			client:     managedRuntime(inspection("exited", false, "healthy", 0)),
			wantConfig: "matches declared fields", wantRun: "exited", wantHealth: "unavailable (container not running)",
			wantFailure: "not running", wantAction: "explicit approval", wantTyped: ActionStart,
		},
		{
			name:       "repeated restarts",
			client:     managedRuntime(inspection("running", true, "healthy", 3)),
			wantConfig: "matches declared fields", wantRun: "running", wantHealth: "healthy",
			wantFailure: "repeated restarts observed (3 total)", wantAction: "no automatic restart", wantTyped: ActionInvestigate,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reports, err := Build(context.Background(), manifest, tc.client)
			if err != nil {
				t.Fatal(err)
			}
			if len(reports) != 1 {
				t.Fatalf("got %d reports, want 1", len(reports))
			}
			got := reports[0]
			if got.Action != tc.wantTyped {
				t.Errorf("action = %q, want %q", got.Action, tc.wantTyped)
			}
			for label, values := range map[string][2]string{
				"configuration":  {got.Configuration, tc.wantConfig},
				"execution":      {got.Execution, tc.wantRun},
				"health":         {got.Health, tc.wantHealth},
				"failure":        {got.Failure, tc.wantFailure},
				"recommendation": {got.Recommendation, tc.wantAction},
			} {
				if !strings.Contains(values[0], values[1]) {
					t.Errorf("%s = %q, want it to contain %q", label, values[0], values[1])
				}
			}
		})
	}
}

func TestBuildReportsRuntimeAndInspectionFailures(t *testing.T) {
	runtimeFailure := fakeRuntime{listErr: errors.New("socket unavailable")}
	reports, err := Build(context.Background(), testManifest(), runtimeFailure)
	if err == nil || !strings.Contains(reports[0].Failure, "socket unavailable") || reports[0].Action != ActionRetryRuntime {
		t.Fatalf("expected explicit runtime failure, reports=%+v err=%v", reports, err)
	}

	inspectionFailure := managedRuntime(runtime.Inspection{})
	inspectionFailure.inspectErr = errors.New("permission denied")
	reports, err = Build(context.Background(), testManifest(), inspectionFailure)
	if err != nil || !strings.Contains(reports[0].Failure, "permission denied") || reports[0].Execution != "running (from container list)" || reports[0].Action != ActionRetryRuntime {
		t.Fatalf("expected conservative inspection failure, reports=%+v err=%v", reports, err)
	}
}

func TestBuildRunningWithoutHealthcheck(t *testing.T) {
	m := testManifest()
	m.Services[0].Healthcheck = nil
	item := inspection("running", true, "", 0)
	item.Config.Healthcheck = nil
	item.State.Healthcheck = nil

	reports, err := Build(context.Background(), m, managedRuntime(item))
	if err != nil {
		t.Fatal(err)
	}
	got := reports[0]
	if got.Configuration != "matches declared fields" || got.Health != "not configured" || !strings.Contains(got.Recommendation, "declaring an application healthcheck") {
		t.Fatalf("unexpected no-healthcheck diagnosis: %+v", got)
	}
}

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{Name: "demo", Services: []manifest.Service{{
		Name:  "web",
		Image: "nginx:stable",
		Healthcheck: &manifest.Healthcheck{
			Command: "true", Interval: "30s", Timeout: "5s", Retries: 3,
		},
	}}}
}

func managedRuntime(item runtime.Inspection) fakeRuntime {
	item.Config.Image = "nginx:stable"
	item.HostConfig.RestartPolicy.Name = "no"
	item.Config.Labels = map[string]string{
		"dev.orisema.managed": "true",
		"dev.orisema.app":     "demo",
		"dev.orisema.service": "web",
	}
	return fakeRuntime{
		containers: []runtime.Container{{Names: []string{"orisema-demo-web"}, State: "running", Image: "nginx:stable"}},
		items:      map[string]runtime.Inspection{"orisema-demo-web": item},
	}
}

func inspection(status string, running bool, health string, restarts int) runtime.Inspection {
	var item runtime.Inspection
	item.State.Status = status
	item.State.Running = running
	item.RestartCount = restarts
	item.Config.Healthcheck = &struct {
		Test     []string `json:"Test"`
		Interval int64    `json:"Interval"`
		Timeout  int64    `json:"Timeout"`
		Retries  int      `json:"Retries"`
	}{Test: []string{"CMD-SHELL", "true"}, Interval: 30_000_000_000, Timeout: 5_000_000_000, Retries: 3}
	item.State.Healthcheck = &struct {
		Status string `json:"Status"`
	}{Status: health}
	return item
}
