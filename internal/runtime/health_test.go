// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"encoding/json"
	"testing"
)

// Synthetic inspect variants cover compatibility and conservative fallbacks.
func TestHealthStatus(t *testing.T) {
	for _, tc := range []struct{ name, config, state, want string }{
		{"running alone", `null`, `{"Running":true,"Status":"running"}`, "not configured"},
		{"disabled with stale health", `{"Test":["NONE"]}`, `{"Running":true,"Status":"running","Health":{"Status":"healthy"}}`, "not configured"},
		{"configured without observation", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"running"}`, "unavailable"},
		{"podman healthy", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"running","Healthcheck":{"Status":"healthy"}}`, "healthy"},
		{"compatible unhealthy", `{"Test":["CMD","true"]}`, `{"Running":true,"Status":"running","Health":{"Status":"unhealthy"}}`, "unhealthy"},
		{"starting", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"running","Healthcheck":{"Status":"starting"}}`, "starting"},
		{"equal fields", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"running","Healthcheck":{"Status":"healthy"},"Health":{"Status":"healthy"}}`, "healthy"},
		{"conflicting fields", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"running","Healthcheck":{"Status":"unhealthy"},"Health":{"Status":"healthy"}}`, "unavailable (conflicting runtime health fields)"},
		{"empty podman field", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"running","Healthcheck":{},"Health":{"Status":"healthy"}}`, "healthy"},
		{"unknown status", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"running","Healthcheck":{"Status":"unknown"}}`, "unavailable"},
		{"stopped stale healthy", `{"Test":["CMD-SHELL","true"]}`, `{"Running":false,"Status":"exited","Healthcheck":{"Status":"healthy"}}`, "unavailable (container not running)"},
		{"paused stale healthy", `{"Test":["CMD-SHELL","true"]}`, `{"Running":true,"Status":"paused","Healthcheck":{"Status":"healthy"}}`, "unavailable (container not running)"},
		{"unknown test", `{"Test":["OTHER","true"]}`, `{"Running":true,"Status":"running","Healthcheck":{"Status":"healthy"}}`, "unavailable"},
		{"empty test", `{"Test":[]}`, `{"Running":true,"Status":"running","Healthcheck":{"Status":"healthy"}}`, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var item Inspection
			if err := json.Unmarshal([]byte(`{"Config":{"Healthcheck":`+tc.config+`},"State":`+tc.state+`}`), &item); err != nil {
				t.Fatal(err)
			}
			if got := item.HealthStatus(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
