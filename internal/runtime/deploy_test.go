// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

// Reuse the test executable as a subprocess to verify actual argv boundaries.
func TestMain(m *testing.M) {
	if os.Getenv("ORISEMA_ARGV_HELPER") == "1" {
		switch os.Args[1] {
		case "ps":
			os.Stdout.WriteString("[]")
		case "run":
			json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		default:
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDeployHealthcheckArguments(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_ARGV_HELPER", "1")
	for _, command := range []string{"nginx -t", "none", `["CMD","false"]`, `printf '%s' "a b"; exit 1`} {
		t.Run(command, func(t *testing.T) {
			svc := manifest.Service{Name: "web", Image: "nginx:1.27", Healthcheck: &manifest.Healthcheck{Command: command, Interval: "30s", Timeout: "5s", Retries: 3}}
			out, err := (Client{Binary: binary}).Deploy(context.Background(), "health-lab", svc)
			if err != nil {
				t.Fatal(err)
			}
			var args []string
			if err := json.Unmarshal([]byte(out), &args); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal([]string{"CMD-SHELL", command})
			want := []string{"run", "--detach", "--name", "orisema-health-lab-web", "--label", "dev.orisema.managed=true", "--label", "dev.orisema.app=health-lab", "--label", "dev.orisema.service=web", "--security-opt", "no-new-privileges", "--health-cmd", string(encoded), "--health-interval", "30s", "--health-timeout", "5s", "--health-retries", "3", "nginx:1.27"}
			if !reflect.DeepEqual(args, want) {
				t.Fatalf("argv = %#v; want %#v", args, want)
			}
		})
	}
}
