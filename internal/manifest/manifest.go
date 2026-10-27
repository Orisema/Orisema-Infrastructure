// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	APIVersion string    `yaml:"apiVersion"`
	Name       string    `yaml:"name"`
	Services   []Service `yaml:"services"`
}

type Service struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
	Ports []Port `yaml:"ports,omitempty"`
	Healthcheck *Healthcheck `yaml:"healthcheck,omitempty"`
	Restart *Restart `yaml:"restart,omitempty"`
	Recovery *Recovery `yaml:"recovery,omitempty"`
}

// Recovery constrains a future explicitly authorized recovery executor. Merely
// declaring it never enables execution.
type Recovery struct {
	MaxAttempts int `yaml:"maxAttempts"`
	MinInterval string `yaml:"minInterval"`
	Cooldown string `yaml:"cooldown"`
}

// Restart describes process-exit recovery. It is independent from application
// healthchecks and intentionally supports only bounded on-failure retries.
type Restart struct {
	Policy string `yaml:"policy"`
	MaxRetries int `yaml:"maxRetries,omitempty"`
}

// Healthcheck uses a shell command executed inside the container.
// Duration fields use Go duration syntax, such as 30s or 1m.
type Healthcheck struct {
 Command string `yaml:"command"`
 Interval string `yaml:"interval"`
 Timeout string `yaml:"timeout"`
 Retries int `yaml:"retries"`
}

type Port struct {
	Host      int `yaml:"host"`
	Container int `yaml:"container"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func Load(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()
	dec := yaml.NewDecoder(io.LimitReader(f, 1<<20+1))
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
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) Validate() error {
	if m.APIVersion != "orisema.dev/v1alpha1" {
		return errors.New("apiVersion must be orisema.dev/v1alpha1")
	}
	if !identifier.MatchString(m.Name) {
		return errors.New("name must be lowercase letters, digits or hyphens and start with a letter (max 63 chars)")
	}
	if len(m.Services) == 0 {
		return errors.New("services must contain at least one service")
	}
	seen := map[string]bool{}
	ports := map[int]bool{}
	for _, s := range m.Services {
		if !identifier.MatchString(s.Name) {
			return fmt.Errorf("invalid service name %q", s.Name)
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate service %q", s.Name)
		}
		seen[s.Name] = true
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
			ports[p.Host] = true
		}
	}
	return nil
}
