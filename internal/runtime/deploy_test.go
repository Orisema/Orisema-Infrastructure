// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

// Reuse the test executable as a subprocess to verify actual argv boundaries.
func TestMain(m *testing.M) {
	if os.Getenv("ORISEMA_ARGV_HELPER") == "1" {
		switch os.Args[1] {
		case "ps":
			_, stateErr := os.Stat(os.Getenv("ORISEMA_RM_MARKER") + ".created")
			if os.Getenv("ORISEMA_RUN_FAIL") == "1" && stateErr == nil {
				os.Stdout.WriteString(`[{"Names":["orisema-health-lab-web"],"State":"created","Image":"nginx:1.27"}]`)
			} else {
				os.Stdout.WriteString("[]")
			}
		case "run":
			if os.Getenv("ORISEMA_RUN_FAIL") == "1" {
				if err := os.WriteFile(os.Getenv("ORISEMA_RM_MARKER")+".created", []byte("created"), 0o600); err != nil {
					os.Exit(3)
				}
				os.Stderr.WriteString("port is already allocated")
				os.Exit(125)
			}
			json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		case "rename":
			json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		case "inspect":
			if os.Getenv("ORISEMA_INSPECT_UNOWNED") == "1" {
				os.Stdout.WriteString(`[{"Name":"orisema-health-lab-web","Config":{"Image":"nginx:1.27","Labels":{}}}]`)
			} else {
				os.Stdout.WriteString(`[{"Name":"orisema-health-lab-web","Config":{"Image":"nginx:1.27","Labels":{"dev.orisema.managed":"true","dev.orisema.app":"health-lab","dev.orisema.service":"web"}}}]`)
			}
		case "rm":
			if path := os.Getenv("ORISEMA_RM_MARKER"); path != "" {
				if err := os.WriteFile(path, []byte(os.Args[len(os.Args)-1]), 0o600); err != nil {
					os.Exit(3)
				}
			}
		default:
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDeployCleansUpOwnedContainerLeftByFailedRun(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := t.TempDir() + "/removed"
	t.Setenv("ORISEMA_ARGV_HELPER", "1")
	t.Setenv("ORISEMA_RUN_FAIL", "1")
	t.Setenv("ORISEMA_RM_MARKER", marker)
	_, err = (Client{Binary: binary}).Deploy(context.Background(), "health-lab", manifest.Service{Name: "web", Image: "nginx:1.27"})
	if err == nil {
		t.Fatal("Deploy() unexpectedly succeeded")
	}
	if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "orisema-health-lab-web" {
		t.Fatalf("safe cleanup marker = %q, %v", got, readErr)
	}
}

func TestDeployDoesNotCleanUpContainerWithoutOwnership(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := t.TempDir() + "/removed"
	t.Setenv("ORISEMA_ARGV_HELPER", "1")
	t.Setenv("ORISEMA_RUN_FAIL", "1")
	t.Setenv("ORISEMA_RM_MARKER", marker)
	t.Setenv("ORISEMA_INSPECT_UNOWNED", "1")
	_, err = (Client{Binary: binary}).Deploy(context.Background(), "health-lab", manifest.Service{Name: "web", Image: "nginx:1.27"})
	if err == nil || !strings.Contains(err.Error(), "cleanup refused") {
		t.Fatalf("Deploy() error = %v; want safe cleanup refusal", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("unowned container removal marker exists: %v", statErr)
	}
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

func TestDeployExplicitHostIPArguments(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_ARGV_HELPER", "1")
	for _, tc := range []struct {
		name   string
		hostIP string
		want   string
	}{
		{name: "IPv4 loopback", hostIP: "127.0.0.1", want: "127.0.0.1:8080:80/tcp"},
		{name: "IPv6 loopback", hostIP: "::1", want: "[::1]:8080:80/tcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := manifest.Service{Name: "web", Image: "nginx:1.27", Ports: []manifest.Port{{Host: 8080, Container: 80, HostIP: tc.hostIP}}}
			out, err := (Client{Binary: binary}).Deploy(context.Background(), "bind-lab", svc)
			if err != nil {
				t.Fatal(err)
			}
			var args []string
			if err := json.Unmarshal([]byte(out), &args); err != nil {
				t.Fatal(err)
			}
			found := false
			for i := range args {
				if i > 0 && args[i-1] == "--publish" && args[i] == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("argv = %#v; want publish %q", args, tc.want)
			}
		})
	}
}

func TestUpdateRuntimeCommandsPreserveArgumentBoundaries(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORISEMA_ARGV_HELPER", "1")
	client := Client{Binary: binary}
	if err := client.RenameContainer(context.Background(), "orisema-app-web", "orisema-rollback-id"); err != nil {
		t.Fatal(err)
	}
	svc := manifest.Service{
		Name: "web", Image: "nginx:1.27",
		Ports:       []manifest.Port{{Host: 8080, Container: 80, HostIP: "127.0.0.1"}},
		Healthcheck: &manifest.Healthcheck{Command: "nginx -t", Interval: "30s", Timeout: "5s", Retries: 3},
		Restart:     &manifest.Restart{Policy: "on-failure", MaxRetries: 2},
	}
	out, err := client.CreateUpdateCandidate(context.Background(), "app", svc)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal([]byte(out), &args); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal([]string{"CMD-SHELL", "nginx -t"})
	want := []string{"run", "--detach", "--name", "orisema-app-web", "--label", "dev.orisema.managed=true", "--label", "dev.orisema.app=app", "--label", "dev.orisema.service=web", "--security-opt", "no-new-privileges", "--health-cmd", string(encoded), "--health-interval", "30s", "--health-timeout", "5s", "--health-retries", "3", "--restart", "on-failure:2", "--publish", "127.0.0.1:8080:80/tcp", "nginx:1.27"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("argv = %#v; want %#v", args, want)
	}
}

func TestUpdateCandidateFailurePerformsNoCleanup(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := t.TempDir() + "/removed"
	t.Setenv("ORISEMA_ARGV_HELPER", "1")
	t.Setenv("ORISEMA_RUN_FAIL", "1")
	t.Setenv("ORISEMA_RM_MARKER", marker)
	_, err = (Client{Binary: binary}).CreateUpdateCandidate(context.Background(), "health-lab", manifest.Service{Name: "web", Image: "nginx:1.27"})
	if err == nil || !strings.Contains(err.Error(), "no cleanup performed") {
		t.Fatalf("CreateUpdateCandidate() error = %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("unexpected cleanup marker: %v", statErr)
	}
}
