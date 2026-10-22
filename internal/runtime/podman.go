// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package runtime reads local Podman state without changing containers.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Container struct {
	Names  []string `json:"Names"`
	State  string   `json:"State"`
	Image  string   `json:"Image"`
}

type Client struct {
	Binary string
}

func (c Client) List(ctx context.Context) ([]Container, error) {
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// No shell and no user-controlled arguments.
	out, err := exec.CommandContext(ctx, bin, "ps", "-a", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("read Podman containers: %w", err)
	}
	var containers []Container
	if err := json.Unmarshal(out, &containers); err != nil {
		return nil, fmt.Errorf("decode Podman output: %w", err)
	}
	return containers, nil
}

func Find(containers []Container, name string) (Container, bool) {
	for _, c := range containers {
		for _, n := range c.Names {
			if n == name {
				return c, true
			}
		}
	}
	return Container{}, false
}
