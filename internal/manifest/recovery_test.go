// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"strings"
	"testing"
)

func TestRecoveryValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		recovery *Recovery
		valid    bool
	}{
		{"omitted", nil, true},
		{"bounded", &Recovery{MaxAttempts: 3, MinInterval: "5m", Cooldown: "30m"}, true},
		{"no attempts", &Recovery{MinInterval: "5m", Cooldown: "30m"}, false},
		{"too many attempts", &Recovery{MaxAttempts: 11, MinInterval: "5m", Cooldown: "30m"}, false},
		{"short interval", &Recovery{MaxAttempts: 3, MinInterval: "500ms", Cooldown: "30m"}, false},
		{"long interval", &Recovery{MaxAttempts: 3, MinInterval: "25h", Cooldown: "30h"}, false},
		{"cooldown below interval", &Recovery{MaxAttempts: 3, MinInterval: "30m", Cooldown: "5m"}, false},
		{"excessive cooldown", &Recovery{MaxAttempts: 3, MinInterval: "5m", Cooldown: "169h"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{APIVersion: "orisema.dev/v1alpha1", Name: "demo", Services: []Service{{Name: "web", Image: "nginx:stable", Recovery: tc.recovery}}}
			err := m.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t, error=%v", tc.valid, err)
			}
			if err != nil && !strings.Contains(err.Error(), "recovery") {
				t.Fatalf("error %q does not identify recovery configuration", err)
			}
		})
	}
}
