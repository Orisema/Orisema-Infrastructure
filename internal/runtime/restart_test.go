// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

func TestDeployRestartArguments(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_ARGV_HELPER", "1")

	for _, tc := range []struct {
		name, policy string
		retries      int
		want         string
	}{
		{"disabled", "no", 0, "no"},
		{"bounded on failure", "on-failure", 3, "on-failure:3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := manifest.Service{Name: "web", Image: "nginx:stable", Restart: &manifest.Restart{Policy: tc.policy, MaxRetries: tc.retries}}
			out, err := (Client{Binary: binary}).Deploy(context.Background(), "restart-lab", svc)
			if err != nil {
				t.Fatal(err)
			}
			var args []string
			if err := json.Unmarshal([]byte(out), &args); err != nil {
				t.Fatal(err)
			}
			if !containsPair(args, "--restart", tc.want) {
				t.Fatalf("argv %#v does not contain --restart %q", args, tc.want)
			}
		})
	}
}

func containsPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
