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
		fmt.Fprintf(stdout, "Proposed deployment for %q (no host inspected; no changes made):\n", m.Name)
		for _, s := range m.Services {
			fmt.Fprintf(stdout, "  - service %q: image %q\n", s.Name, s.Image)
			for _, p := range s.Ports {
				fmt.Fprintf(stdout, "      port: %d -> %d/tcp\n", p.Host, p.Container)
			}
		}
		fmt.Fprintln(stdout, "Note: this is a manifest preview, not a host state diff or executable deployment plan.")
	case "deploy":
		containers, err := (runtime.Client{}).List(context.Background())
		if err != nil { fmt.Fprintf(stderr, "error: %v\n", err); return 1 }
		for _, svc := range m.Services {
			name := "orisema-" + m.Name + "-" + svc.Name
			if _, found := runtime.Find(containers, name); found {
				fmt.Fprintf(stderr, "error: %s already exists; no changes made\n", name)
				return 1
			}
		}
		fmt.Fprintf(stdout, "Create-only Podman deployment for %q:\n", m.Name)
		for _, svc := range m.Services {
			fmt.Fprintf(stdout, "  create orisema-%s-%s using %s\n", m.Name, svc.Name, svc.Image)
			for _, p := range svc.Ports { fmt.Fprintf(stdout, "    publish 0.0.0.0:%d -> %d/tcp\n", p.Host, p.Container) }
		}
		fmt.Fprintln(stdout, "Images may be downloaded. No existing containers will be replaced. This does NOT configure firewalls, health checks, persistence, or rollback.")
		fmt.Fprint(stdout, "Type DEPLOY to continue: ")
		reader := bufio.NewReader(stdin)
		answer, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(answer) != "DEPLOY" { fmt.Fprintln(stderr, "cancelled"); return 1 }
		if strings.TrimSpace(answer) != "DEPLOY" { fmt.Fprintln(stderr, "cancelled"); return 1 }
		for _, svc := range m.Services {
			id, err := (runtime.Client{}).Deploy(context.Background(), m.Name, svc)
			if err != nil { fmt.Fprintf(stderr, "deployment stopped: %v\nPreviously created services, if any, were not rolled back.\n", err); return 1 }
			fmt.Fprintf(stdout, "created orisema-%s-%s: %s\n", m.Name, svc.Name, id)
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
