// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/networks"
	"github.com/Orisema/Orisema-Infrastructure/internal/publicconfig"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
	"github.com/Orisema/Orisema-Infrastructure/internal/secrets"
)

const (
	Noop  Action = "NOOP"
	Drift Action = "DRIFT"
)

type Inspector interface {
	Inspect(context.Context, string) (runtime.Inspection, error)
}

// BuildInspected checks declared managed fields only. It never mutates Podman.
// Inspection failures are REVIEW, not evidence of safe equivalence.
func BuildInspected(ctx context.Context, m *manifest.Manifest, existing []runtime.Container, inspector Inspector) []Step {
	return buildInspected(ctx, m, existing, inspector, nil, nil, false, false)
}

// BuildInspectedResolved includes externally resolved secret identities in the
// comparison without reading or transporting their values.
func BuildInspectedResolved(ctx context.Context, m *manifest.Manifest, existing []runtime.Container, inspector Inspector, resolved map[string][]secrets.ResolvedMount) []Step {
	return buildInspected(ctx, m, existing, inspector, resolved, nil, true, false)
}

// BuildInspectedResolvedRuntime includes secret and external-network identities
// plus explicitly public environment configuration in conservative comparison.
func BuildInspectedResolvedRuntime(ctx context.Context, m *manifest.Manifest, existing []runtime.Container, inspector Inspector, resolvedSecrets map[string][]secrets.ResolvedMount, resolvedNetworks map[string][]networks.ResolvedBinding) []Step {
	return buildInspected(ctx, m, existing, inspector, resolvedSecrets, resolvedNetworks, true, true)
}

func buildInspected(ctx context.Context, m *manifest.Manifest, existing []runtime.Container, inspector Inspector, resolvedSecrets map[string][]secrets.ResolvedMount, resolvedNetworks map[string][]networks.ResolvedBinding, allowSecrets, allowAlpha4 bool) []Step {
	steps := make([]Step, 0, len(m.Services))
	for _, svc := range m.Services {
		name := "orisema-" + m.Name + "-" + svc.Name
		mounts := resolvedSecrets[svc.Name]
		networkBindings := resolvedNetworks[svc.Name]
		if (len(svc.PublicEnvironment) != 0 || len(svc.Networks) != 0) && !allowAlpha4 {
			steps = append(steps, Step{Name: name, Image: svc.Image, Action: Review, Reason: "public configuration or network delivery is declared but not implemented; no changes made"})
			continue
		}
		if len(svc.Networks) != len(networkBindings) {
			steps = append(steps, Step{Name: name, Image: svc.Image, Action: Review, Reason: "network delivery metadata is incomplete; no changes made"})
			continue
		}
		if len(svc.Secrets) != 0 && (!allowSecrets || len(mounts) != len(svc.Secrets)) {
			steps = append(steps, Step{Name: name, Image: svc.Image, Action: Review, Reason: "secret delivery is declared but not implemented; no changes made"})
			continue
		}
		_, exists := runtime.Find(existing, name)
		if !exists {
			steps = append(steps, Step{Name: name, Image: svc.Image, Action: Create, Reason: "container not found"})
			continue
		}
		inspected, err := inspector.Inspect(ctx, name)
		if err != nil {
			steps = append(steps, Step{Name: name, Image: svc.Image, Action: Review, Reason: fmt.Sprintf("inspect failed: %v", err)})
			continue
		}
		labels := inspected.Config.Labels
		if labels["dev.orisema.managed"] != "true" || labels["dev.orisema.app"] != m.Name || labels["dev.orisema.service"] != svc.Name {
			steps = append(steps, Step{Name: name, Image: svc.Image, Action: Review, Reason: "ownership labels missing or mismatched"})
			continue
		}
		reasons := []string{}
		expectedNames, expectedHash := publicconfig.Metadata(svc.PublicEnvironment)
		if len(svc.PublicEnvironment) != 0 {
			if labels[publicconfig.NamesLabel] != expectedNames || labels[publicconfig.HashLabel] != expectedHash {
				reasons = append(reasons, "public configuration metadata differs")
			}
			actualEnvironment := publicconfig.EnvironmentMap(inspected.Config.Env)
			for _, entry := range svc.PublicEnvironment {
				actual, ok := actualEnvironment[entry.Name]
				if !ok || actual != entry.Value {
					reasons = append(reasons, "public configuration differs")
					break
				}
			}
		} else if labels[publicconfig.NamesLabel] != "" || labels[publicconfig.HashLabel] != "" {
			reasons = append(reasons, "unexpected public configuration metadata")
		}
		expectedNetworkLabels := make(map[string]string, len(networkBindings))
		expectedNetworkNames := make(map[string]bool, len(networkBindings))
		for _, binding := range networkBindings {
			expectedNetworkLabels["dev.orisema.network."+binding.Name] = string(binding.Provider) + ":" + binding.Identity + ":" + binding.Driver
			expectedNetworkNames[binding.Locator] = true
		}
		for key, expected := range expectedNetworkLabels {
			if labels[key] != expected {
				reasons = append(reasons, "network binding metadata differs")
				break
			}
		}
		for key := range labels {
			if strings.HasPrefix(key, "dev.orisema.network.") {
				if _, ok := expectedNetworkLabels[key]; !ok {
					reasons = append(reasons, "unexpected network binding metadata")
					break
				}
			}
		}
		if len(networkBindings) != 0 {
			actualNetworks := inspected.NetworkSettings.Networks
			if len(actualNetworks) != len(expectedNetworkNames) {
				reasons = append(reasons, "network attachment count differs")
			} else {
				for locator := range expectedNetworkNames {
					if _, ok := actualNetworks[locator]; !ok {
						reasons = append(reasons, "network attachment differs")
						break
					}
				}
			}
		}
		expectedSecretLabels := make(map[string]string, len(mounts))
		for _, mount := range mounts {
			expectedSecretLabels["dev.orisema.secret."+mount.Target] = string(mount.Provider) + ":" + mount.Identity
		}
		for key, expected := range expectedSecretLabels {
			if labels[key] != expected {
				reasons = append(reasons, "secret binding metadata differs")
				break
			}
		}
		for key := range labels {
			if strings.HasPrefix(key, "dev.orisema.secret.") {
				if _, ok := expectedSecretLabels[key]; !ok {
					reasons = append(reasons, "unexpected secret binding metadata")
					break
				}
			}
		}
		if inspected.Config.Image != svc.Image {
			reasons = append(reasons, "configured image differs")
		}
		expected := make(map[string]string, len(svc.Ports))
		for _, p := range svc.Ports {
			expected[strconv.Itoa(p.Container)+"/tcp"] = strconv.Itoa(p.Host)
		}
		actual := inspected.HostConfig.PortBindings
		if len(actual) != len(expected) {
			reasons = append(reasons, "published port count differs")
		} else {
			for containerPort, hostPort := range expected {
				bindings, ok := actual[containerPort]
				expectedIP := ""
				for _, declared := range svc.Ports {
					if strconv.Itoa(declared.Container)+"/tcp" == containerPort {
						expectedIP = declared.HostIP
						break
					}
				}
				ipMatches := false
				if len(bindings) == 1 {
					ipMatches = hostIPMatches(expectedIP, bindings[0].HostIP)
				}
				if !ok || len(bindings) != 1 || bindings[0].HostPort != hostPort || !ipMatches {
					reasons = append(reasons, "published port binding differs")
					break
				}
			}
		}
		if h := svc.Healthcheck; h != nil {
			actualHealth := inspected.Config.Healthcheck
			if actualHealth == nil {
				reasons = append(reasons, "healthcheck missing")
			} else {
				interval, _ := time.ParseDuration(h.Interval)
				timeout, _ := time.ParseDuration(h.Timeout)
				test := actualHealth.Test
				commandMatches := len(test) == 2 && test[0] == "CMD-SHELL" && test[1] == h.Command
				if !commandMatches || actualHealth.Interval != int64(interval) || actualHealth.Timeout != int64(timeout) || actualHealth.Retries != h.Retries {
					reasons = append(reasons, "healthcheck configuration differs")
				}
			}
		} else if h := inspected.Config.Healthcheck; h != nil && !(len(h.Test) == 1 && h.Test[0] == "NONE") {
			reasons = append(reasons, "unexpected healthcheck configured")
		}
		expectedRestartName := "no"
		expectedRestartRetries := 0
		if r := svc.Restart; r != nil {
			expectedRestartName = r.Policy
			expectedRestartRetries = r.MaxRetries
		}
		actualRestart := inspected.HostConfig.RestartPolicy
		actualRestartName := actualRestart.Name
		// Podman 4.9.3 has been observed to report the default policy as either
		// an empty name (rootful) or "no" (rootless). Both mean no restart.
		if actualRestartName == "" {
			actualRestartName = "no"
		}
		if actualRestartName != expectedRestartName || actualRestart.MaximumRetryCount != expectedRestartRetries {
			reasons = append(reasons, "restart policy differs")
		}
		if len(reasons) > 0 {
			steps = append(steps, Step{Name: name, Image: svc.Image, CurrentImage: inspected.Config.Image, CurrentID: inspected.ID, Action: Drift, Differences: append([]string(nil), reasons...), TypedDifferences: classifyDifferences(reasons), Reason: strings.Join(reasons, ", ")})
		} else {
			steps = append(steps, Step{Name: name, Image: svc.Image, CurrentImage: inspected.Config.Image, CurrentID: inspected.ID, Action: Noop, Reason: "managed image reference, ports, healthcheck, restart policy, secret metadata, public configuration and network bindings match; live health and other settings not verified"})
		}
	}
	return steps
}

func hostIPMatches(expected, actual string) bool {
	if expected == "" {
		return actual == "" || actual == "0.0.0.0"
	}
	expectedIP := net.ParseIP(expected)
	actualIP := net.ParseIP(actual)
	return expectedIP != nil && actualIP != nil && expectedIP.Equal(actualIP)
}
