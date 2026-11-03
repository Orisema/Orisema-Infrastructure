// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package networks

import (
	"fmt"
	"os/exec"
	"strings"
)

func createIntegrationNetwork(name string) error {
	out, err := exec.Command("podman", "network", "create", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create fixture network: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeIntegrationNetwork(name string) error {
	out, err := exec.Command("podman", "network", "rm", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove fixture network: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func integrationNetworkExists(name string) error {
	out, err := exec.Command("podman", "network", "exists", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("fixture network missing: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
