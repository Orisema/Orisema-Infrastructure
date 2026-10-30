// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Binding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}
type Inspection struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	State struct {
		Status      string `json:"Status"`
		Running     bool   `json:"Running"`
		ExitCode    int    `json:"ExitCode"`
		StartedAt   string `json:"StartedAt"`
		FinishedAt  string `json:"FinishedAt"`
		Healthcheck *struct {
			Status string `json:"Status"`
		} `json:"Healthcheck"`
		Health *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	RestartCount int `json:"RestartCount"`
	Config       struct {
		Image       string            `json:"Image"`
		Labels      map[string]string `json:"Labels"`
		Healthcheck *struct {
			Test     []string `json:"Test"`
			Interval int64    `json:"Interval"`
			Timeout  int64    `json:"Timeout"`
			Retries  int      `json:"Retries"`
		} `json:"Healthcheck"`
	} `json:"Config"`
	HostConfig struct {
		PortBindings  map[string][]Binding `json:"PortBindings"`
		RestartPolicy struct {
			Name              string `json:"Name"`
			MaximumRetryCount int    `json:"MaximumRetryCount"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
}

// Inspect reads a container's configured image reference, ownership labels,
// and host port bindings. No mutation is performed.
func (c Client) Inspect(ctx context.Context, name string) (Inspection, error) {
	bin := c.Binary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "inspect", "--type", "container", name).Output()
	if err != nil {
		return Inspection{}, fmt.Errorf("inspect container %q: %w", name, err)
	}
	var items []Inspection
	if err := json.Unmarshal(out, &items); err != nil {
		return Inspection{}, fmt.Errorf("decode inspect output: %w", err)
	}
	if len(items) != 1 {
		return Inspection{}, fmt.Errorf("expected one inspected container, got %d", len(items))
	}
	return items[0], nil
}
