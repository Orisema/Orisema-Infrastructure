// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"bufio"
	"strings"
	"fmt"
	"io"
	"os"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/planner"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "validate" && args[0] != "plan" && args[0] != "status" && args[0] != "deploy") {
		fmt.Fprintln(stderr, "Usage: orisema <validate|plan|status|deploy> <manifest.yaml>")
		return 2
	}
	m, err := manifest.Load(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	switch args[0] {
	case "validate":
		fmt.Fprintf(stdout, "valid: %s (%d services)\n", m.Name, len(m.Services))
	case "plan":
		containers, err := (runtime.Client{}).List(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Read-only Podman plan for %q:\n", m.Name)
		for _, step := range planner.BuildInspected(context.Background(), m, containers, runtime.Client{}) {
			fmt.Fprintf(stdout, "  [%s] %s: %s (%s)\n", step.Action, step.Name, step.Image, step.Reason)
		}
		fmt.Fprintln(stdout, "NOOP checks only ownership labels, configured image reference and ports; health and other settings not verified. No changes made.")
	case "deploy":
		client := runtime.Client{}
		containers, err := client.List(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		steps := planner.BuildInspected(context.Background(), m, containers, client)
		creates := 0
		blocked := false
		fmt.Fprintf(stdout, "Conservative deployment plan for %q:\n", m.Name)
		for _, step := range steps {
			fmt.Fprintf(stdout, "  [%s] %s (%s)\n", step.Action, step.Name, step.Reason)
			switch step.Action {
			case planner.Create:
				creates++
			case planner.Noop:
				// No action: matching declared fields only.
			default:
				blocked = true
			}
		}
		if blocked {
			fmt.Fprintln(stderr, "deployment blocked: DRIFT or REVIEW requires manual investigation; no changes made")
			return 1
		}
		if creates == 0 {
			fmt.Fprintln(stdout, "No containers to create. No changes made. Note: runtime health and undeclared settings are not verified.")
			return 0
		}
		fmt.Fprintln(stdout, "Only CREATE services will be started. Existing containers will not be changed.")
		fmt.Fprintln(stdout, "Images may be downloaded; published ports bind to all interfaces. No rollback or health verification.")
		fmt.Fprint(stdout, "Type DEPLOY to continue: ")
		reader := bufio.NewReader(stdin)
		answer, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(answer) != "DEPLOY" {
			fmt.Fprintln(stderr, "cancelled")
			return 1
		}
		if strings.TrimSpace(answer) != "DEPLOY" {
			fmt.Fprintln(stderr, "cancelled")
			return 1
		}
		for i, step := range steps {
			if step.Action != planner.Create {
				continue
			}
			id, err := client.Deploy(context.Background(), m.Name, m.Services[i])
			if err != nil {
				fmt.Fprintf(stderr, "deployment stopped: %v\nPreviously created services, if any, were not rolled back.\n", err)
				return 1
			}
			fmt.Fprintf(stdout, "created %s: %s\n", step.Name, id)
		}
	case "status":
		containers, err := (runtime.Client{}).List(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Podman status for %q (read-only):\n", m.Name)
		for _, svc := range m.Services {
			name := "orisema-" + m.Name + "-" + svc.Name
			if found, ok := runtime.Find(containers, name); ok {
				fmt.Fprintf(stdout, "  %s: %s (image: %s)\n", name, found.State, found.Image)
			} else {
				fmt.Fprintf(stdout, "  %s: not found\n", name)
			}
		}
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
