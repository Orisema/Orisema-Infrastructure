// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

func TestInspectedHealthcheck(t *testing.T) {
	for _, tc := range []struct {
		name, health string
		declared     bool
		want         Action
	}{
		{"shell equivalent", `{"Test":["CMD-SHELL","nginx -t"],"Interval":30000000000,"Timeout":5000000000,"Retries":3}`, true, Noop},
		{"exec is different", `{"Test":["CMD","nginx -t"],"Interval":30000000000,"Timeout":5000000000,"Retries":3}`, true, Drift},
		{"different command", `{"Test":["CMD-SHELL","false"],"Interval":30000000000,"Timeout":5000000000,"Retries":3}`, true, Drift},
		{"different interval", `{"Test":["CMD-SHELL","nginx -t"],"Interval":1000000000,"Timeout":5000000000,"Retries":3}`, true, Drift},
		{"different timeout", `{"Test":["CMD-SHELL","nginx -t"],"Interval":30000000000,"Timeout":1000000000,"Retries":3}`, true, Drift},
		{"different retries", `{"Test":["CMD-SHELL","nginx -t"],"Interval":30000000000,"Timeout":5000000000,"Retries":1}`, true, Drift},
		{"missing", `null`, true, Drift},
		{"disabled", `{"Test":["NONE"]}`, true, Drift},
		{"absent", `null`, false, Noop},
		{"explicitly disabled", `{"Test":["NONE"]}`, false, Noop},
		{"unknown empty test", `{"Test":[]}`, false, Drift},
		{"inherited", `{"Test":["CMD-SHELL","true"]}`, false, Drift},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var item runtime.Inspection
			if err := json.Unmarshal([]byte(`{"Config":{"Healthcheck":`+tc.health+`}}`), &item); err != nil {
				t.Fatal(err)
			}
			item.Config.Image = "nginx:1.27"
			item.Config.Labels = map[string]string{"dev.orisema.managed": "true", "dev.orisema.app": "health-lab", "dev.orisema.service": "web"}
			svc := manifest.Service{Name: "web", Image: item.Config.Image}
			if tc.declared {
				svc.Healthcheck = &manifest.Healthcheck{Command: "nginx -t", Interval: "0.5m", Timeout: "5000ms", Retries: 3}
			}
			m := &manifest.Manifest{Name: "health-lab", Services: []manifest.Service{svc}}
			existing := []runtime.Container{{Names: []string{"orisema-health-lab-web"}}}
			got := BuildInspected(context.Background(), m, existing, fakeInspector{item: item})
			if len(got) != 1 || got[0].Action != tc.want {
				t.Fatalf("got %#v, want %s", got, tc.want)
			}
			failed := BuildInspected(context.Background(), m, existing, fakeInspector{err: errors.New("inspect failed")})
			if failed[0].Action != Review {
				t.Fatalf("inspection error must be REVIEW: %#v", failed)
			}
		})
	}
}
