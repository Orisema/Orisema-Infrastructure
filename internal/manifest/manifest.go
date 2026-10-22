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
