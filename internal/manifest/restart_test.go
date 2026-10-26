// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"strings"
	"testing"
)

func TestRestartValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		restart *Restart
		valid   bool
	}{
		{"omitted", nil, true},
		{"disabled", &Restart{Policy: "no"}, true},
		{"bounded on failure", &Restart{Policy: "on-failure", MaxRetries: 3}, true},
		{"missing policy", &Restart{}, false},
		{"unbounded on failure", &Restart{Policy: "on-failure"}, false},
		{"excessive retries", &Restart{Policy: "on-failure", MaxRetries: 101}, false},
		{"retries with no", &Restart{Policy: "no", MaxRetries: 3}, false},
		{"always excluded", &Restart{Policy: "always"}, false},
		{"unless stopped excluded", &Restart{Policy: "unless-stopped"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{APIVersion: "orisema.dev/v1alpha1", Name: "demo", Services: []Service{{Name: "web", Image: "nginx:stable", Restart: tc.restart}}}
			err := m.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t, error=%v", tc.valid, err)
			}
			if err != nil && !strings.Contains(err.Error(), "restart") {
				t.Fatalf("error %q does not identify restart configuration", err)
			}
		})
	}
}
