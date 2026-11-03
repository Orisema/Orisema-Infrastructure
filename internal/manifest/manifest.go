// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	APIVersion     string              `yaml:"apiVersion"`
	Kind           string              `yaml:"kind,omitempty"`
	Name           string              `yaml:"name"`
	Secrets        []SecretDeclaration `yaml:"secrets,omitempty"`
	Services       []Service           `yaml:"services"`
	secretsPresent bool
}

type Service struct {
	Name           string          `yaml:"name"`
	Image          string          `yaml:"image"`
	Ports          []Port          `yaml:"ports,omitempty"`
	Healthcheck    *Healthcheck    `yaml:"healthcheck,omitempty"`
	Restart        *Restart        `yaml:"restart,omitempty"`
	Recovery       *Recovery       `yaml:"recovery,omitempty"`
	Secrets        []SecretBinding `yaml:"secrets,omitempty"`
	secretsPresent bool
}

type SecretDeclaration struct {
	Name   string       `yaml:"name"`
	Source SecretSource `yaml:"source"`
}

type SecretSource struct {
	Podman *PodmanSecretSource `yaml:"podman,omitempty"`
	File   *FileSecretSource   `yaml:"file,omitempty"`
}

type PodmanSecretSource struct {
	Name string `yaml:"name"`
}

type FileSecretSource struct {
	Path string `yaml:"path"`
}

type SecretBinding struct {
	Name   string `yaml:"name"`
	Target string `yaml:"target"`
}

// Recovery constrains a future explicitly authorized recovery executor. Merely
// declaring it never enables execution.
type Recovery struct {
	MaxAttempts int    `yaml:"maxAttempts"`
	MinInterval string `yaml:"minInterval"`
	Cooldown    string `yaml:"cooldown"`
}

// Restart describes process-exit recovery. It is independent from application
// healthchecks and intentionally supports only bounded on-failure retries.
type Restart struct {
	Policy     string `yaml:"policy"`
	MaxRetries int    `yaml:"maxRetries,omitempty"`
}

// Healthcheck uses a shell command executed inside the container.
// Duration fields use Go duration syntax, such as 30s or 1m.
type Healthcheck struct {
	Command  string `yaml:"command"`
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
	Retries  int    `yaml:"retries"`
}

type Port struct {
	Host      int    `yaml:"host"`
	Container int    `yaml:"container"`
	HostIP    string `yaml:"hostIP,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func Load(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if len(data) > 1<<20 {
		return nil, errors.New("manifest exceeds 1 MiB limit")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple YAML documents are not supported")
		}
		return nil, fmt.Errorf("parse trailing YAML: %w", err)
	}
	markSecretFieldPresence(data, &m)
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) Validate() error {
	loopbackPorts := false
	alpha3 := false
	switch m.APIVersion {
	case "orisema.dev/v1alpha1":
		if m.Kind != "" {
			return errors.New("kind is not part of orisema.dev/v1alpha1; migrate the manifest to orisema.dev/v1alpha2")
		}
	case "orisema.dev/v1alpha2":
		loopbackPorts = true
		if m.Kind != "Application" {
			return errors.New("kind must be Application for orisema.dev/v1alpha2")
		}
	case "orisema.dev/v1alpha3":
		loopbackPorts = true
		alpha3 = true
		if m.Kind != "Application" {
			return errors.New("kind must be Application for orisema.dev/v1alpha3")
		}
	default:
		return errors.New("apiVersion must be orisema.dev/v1alpha1, orisema.dev/v1alpha2 or orisema.dev/v1alpha3")
	}
	if !alpha3 && (m.secretsPresent || len(m.Secrets) != 0) {
		return fmt.Errorf("secrets are not part of %s; migrate the manifest to orisema.dev/v1alpha3", m.APIVersion)
	}
	if !identifier.MatchString(m.Name) {
		return errors.New("name must be lowercase letters, digits or hyphens and start with a letter (max 63 chars)")
	}
	if len(m.Services) == 0 {
		return errors.New("services must contain at least one service")
	}
	if len(m.Secrets) > 64 {
		return errors.New("secrets must contain at most 64 declarations")
	}
	declaredSecrets := make(map[string]bool, len(m.Secrets))
	for _, secret := range m.Secrets {
		if !identifier.MatchString(secret.Name) {
			return fmt.Errorf("invalid secret name %q", secret.Name)
		}
		if declaredSecrets[secret.Name] {
			return fmt.Errorf("duplicate secret %q", secret.Name)
		}
		declaredSecrets[secret.Name] = true
		sources := 0
		if secret.Source.Podman != nil {
			sources++
			if !identifier.MatchString(secret.Source.Podman.Name) {
				return fmt.Errorf("secret %q: invalid podman source name", secret.Name)
			}
		}
		if secret.Source.File != nil {
			sources++
			path := secret.Source.File.Path
			if path == "" || strings.ContainsRune(path, '\x00') || !filepath.IsAbs(path) || filepath.Clean(path) != path {
				return fmt.Errorf("secret %q: file source path must be clean and absolute", secret.Name)
			}
		}
		if sources != 1 {
			return fmt.Errorf("secret %q: source must contain exactly one of podman or file", secret.Name)
		}
	}
	seen := map[string]bool{}
	ports := map[int]bool{}
	for _, s := range m.Services {
		containerPorts := map[int]bool{}
		if !identifier.MatchString(s.Name) {
			return fmt.Errorf("invalid service name %q", s.Name)
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate service %q", s.Name)
		}
		seen[s.Name] = true
		if !alpha3 && (s.secretsPresent || len(s.Secrets) != 0) {
			return fmt.Errorf("service %q: secrets are not part of %s; migrate the manifest to orisema.dev/v1alpha3", s.Name, m.APIVersion)
		}
		if len(s.Secrets) > 64 {
			return fmt.Errorf("service %q: secrets must contain at most 64 bindings", s.Name)
		}
		boundSecrets := make(map[string]bool, len(s.Secrets))
		targets := make(map[string]bool, len(s.Secrets))
		for _, binding := range s.Secrets {
			if !identifier.MatchString(binding.Name) || !declaredSecrets[binding.Name] {
				return fmt.Errorf("service %q: secret %q is not declared", s.Name, binding.Name)
			}
			if boundSecrets[binding.Name] {
				return fmt.Errorf("service %q: secret %q is bound more than once", s.Name, binding.Name)
			}
			if !identifier.MatchString(binding.Target) {
				return fmt.Errorf("service %q: secret %q target must be a valid file name", s.Name, binding.Name)
			}
			if targets[binding.Target] {
				return fmt.Errorf("service %q: secret target %q is used more than once", s.Name, binding.Target)
			}
			boundSecrets[binding.Name] = true
			targets[binding.Target] = true
		}
		if strings.TrimSpace(s.Image) == "" || strings.ContainsAny(s.Image, " \t\n\r") {
			return fmt.Errorf("service %q: image must be a nonempty reference without whitespace", s.Name)
		}
		if h := s.Healthcheck; h != nil {
			if strings.TrimSpace(h.Command) == "" {
				return fmt.Errorf("service %q: healthcheck command is required", s.Name)
			}
			if strings.ContainsAny(h.Command, "\x00\n\r") {
				return fmt.Errorf("service %q: healthcheck command must be a single line without NUL", s.Name)
			}
			interval, err := time.ParseDuration(h.Interval)
			if err != nil || interval < time.Second {
				return fmt.Errorf("service %q: healthcheck interval must be at least 1s", s.Name)
			}
			timeout, err := time.ParseDuration(h.Timeout)
			if err != nil || timeout < time.Second {
				return fmt.Errorf("service %q: healthcheck timeout must be at least 1s", s.Name)
			}
			if h.Retries < 1 || h.Retries > 100 {
				return fmt.Errorf("service %q: healthcheck retries must be between 1 and 100", s.Name)
			}
		}
		if r := s.Restart; r != nil {
			switch r.Policy {
			case "no":
				if r.MaxRetries != 0 {
					return fmt.Errorf("service %q: restart maxRetries is only valid with on-failure", s.Name)
				}
			case "on-failure":
				if r.MaxRetries < 1 || r.MaxRetries > 100 {
					return fmt.Errorf("service %q: restart maxRetries must be between 1 and 100 for on-failure", s.Name)
				}
			default:
				return fmt.Errorf("service %q: restart policy must be no or on-failure", s.Name)
			}
		}
		if r := s.Recovery; r != nil {
			if r.MaxAttempts < 1 || r.MaxAttempts > 10 {
				return fmt.Errorf("service %q: recovery maxAttempts must be between 1 and 10", s.Name)
			}
			minInterval, err := time.ParseDuration(r.MinInterval)
			if err != nil || minInterval < time.Second || minInterval > 24*time.Hour {
				return fmt.Errorf("service %q: recovery minInterval must be between 1s and 24h", s.Name)
			}
			cooldown, err := time.ParseDuration(r.Cooldown)
			if err != nil || cooldown < minInterval || cooldown > 7*24*time.Hour {
				return fmt.Errorf("service %q: recovery cooldown must be at least minInterval and at most 168h", s.Name)
			}
		}
		for _, p := range s.Ports {
			if p.Host < 1 || p.Host > 65535 || p.Container < 1 || p.Container > 65535 {
				return fmt.Errorf("service %q: ports must be between 1 and 65535", s.Name)
			}
			if ports[p.Host] {
				return fmt.Errorf("host port %d is used more than once", p.Host)
			}
			if loopbackPorts {
				if containerPorts[p.Container] {
					return fmt.Errorf("service %q: %s container port %d is published more than once", s.Name, m.APIVersion, p.Container)
				}
				ip := net.ParseIP(p.HostIP)
				if ip == nil || !ip.IsLoopback() {
					return fmt.Errorf("service %q: %s port hostIP must be an explicit loopback IP (127.0.0.0/8 or ::1)", s.Name, m.APIVersion)
				}
			} else if p.HostIP != "" {
				return fmt.Errorf("service %q: hostIP is not part of v1alpha1; migrate the manifest to orisema.dev/v1alpha2", s.Name)
			}
			ports[p.Host] = true
			containerPorts[p.Container] = true
		}
	}
	return nil
}

func markSecretFieldPresence(data []byte, m *Manifest) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil || len(document.Content) == 0 {
		return
	}
	root := document.Content[0]
	m.secretsPresent = mappingHasKey(root, "secrets")
	services := mappingValue(root, "services")
	if services == nil || services.Kind != yaml.SequenceNode {
		return
	}
	for i, node := range services.Content {
		if i >= len(m.Services) {
			break
		}
		m.Services[i].secretsPresent = mappingHasKey(node, "secrets")
	}
}

func mappingHasKey(node *yaml.Node, key string) bool {
	return mappingValue(node, key) != nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
