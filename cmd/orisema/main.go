// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || (args[0] != "validate" && args[0] != "plan") {
		fmt.Fprintln(stderr, "Usage: orisema <validate|plan> <manifest.yaml>")
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
	default:
		panic(errors.New("unreachable command"))
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
