// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "validate" && args[0] != "plan" && args[0] != "status") {
		fmt.Fprintln(stderr, "Usage: orisema <validate|plan|status> <manifest.yaml>")
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
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
