// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const registryAPIVersion = "orisema.dev/hosts/v1alpha1"

type Registry struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Hosts      []Host `yaml:"hosts"`
}
type Host struct {
	Name           string `yaml:"name"`
	Address        string `yaml:"address"`
	Port           int    `yaml:"port"`
	User           string `yaml:"user"`
	IdentityFile   string `yaml:"identityFile"`
	KnownHostsFile string `yaml:"knownHostsFile"`
	ProxyJump      string `yaml:"proxyJump,omitempty"`
	ConnectTimeout string `yaml:"connectTimeout,omitempty"`
	CommandTimeout string `yaml:"commandTimeout,omitempty"`
}

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var userPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
var hostnamePattern = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)

func LoadRegistry(path string) (*Registry, error) {
	data, err := readPrivateFile(path, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("load host registry: %w", err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	var registry Registry
	if err := dec.Decode(&registry); err != nil {
		return nil, fmt.Errorf("parse host registry: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("host registry must contain exactly one YAML document")
	}
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	return &registry, nil
}

func (r *Registry) Validate() error {
	if r.APIVersion != registryAPIVersion || r.Kind != "HostRegistry" || len(r.Hosts) == 0 {
		return errors.New("host registry version, kind or hosts are invalid")
	}
	byName := map[string]Host{}
	for i := range r.Hosts {
		h := &r.Hosts[i]
		if !aliasPattern.MatchString(h.Name) || !validAddress(h.Address) || !userPattern.MatchString(h.User) || h.Port < 1 || h.Port > 65535 {
			return fmt.Errorf("host %d has invalid identity or address fields", i)
		}
		if _, ok := byName[h.Name]; ok {
			return fmt.Errorf("duplicate host %q", h.Name)
		}
		if err := validateCredentialFile(h.IdentityFile, "identity"); err != nil {
			return fmt.Errorf("host %s: %w", h.Name, err)
		}
		if err := validateCredentialFile(h.KnownHostsFile, "known_hosts"); err != nil {
			return fmt.Errorf("host %s: %w", h.Name, err)
		}
		connect, err := boundedDuration(h.ConnectTimeout, 10*time.Second, 1*time.Second, 30*time.Second)
		if err != nil {
			return fmt.Errorf("host %s connectTimeout: %w", h.Name, err)
		}
		command, err := boundedDuration(h.CommandTimeout, 30*time.Second, 1*time.Second, 2*time.Minute)
		if err != nil {
			return fmt.Errorf("host %s commandTimeout: %w", h.Name, err)
		}
		h.ConnectTimeout = connect.String()
		h.CommandTimeout = command.String()
		byName[h.Name] = *h
	}
	for _, h := range r.Hosts {
		if h.ProxyJump != "" {
			if !aliasPattern.MatchString(h.ProxyJump) || h.ProxyJump == h.Name {
				return fmt.Errorf("host %s has invalid proxyJump", h.Name)
			}
			if _, ok := byName[h.ProxyJump]; !ok {
				return fmt.Errorf("host %s references unknown proxyJump", h.Name)
			}
		}
		seen := map[string]bool{h.Name: true}
		current := h
		for depth := 0; current.ProxyJump != ""; depth++ {
			if depth >= 3 {
				return fmt.Errorf("host %s proxy chain exceeds three hops", h.Name)
			}
			if seen[current.ProxyJump] {
				return fmt.Errorf("host %s proxy chain contains a cycle", h.Name)
			}
			seen[current.ProxyJump] = true
			current = byName[current.ProxyJump]
		}
	}
	return nil
}

func (r Registry) Host(name string) (Host, bool) {
	for _, h := range r.Hosts {
		if h.Name == name {
			return h, true
		}
	}
	return Host{}, false
}
func (r Registry) chain(name string) ([]Host, error) {
	h, ok := r.Host(name)
	if !ok {
		return nil, fmt.Errorf("host %q is not registered", name)
	}
	result := []Host{h}
	for h.ProxyJump != "" {
		h, _ = r.Host(h.ProxyJump)
		result = append(result, h)
	}
	sort.SliceStable(result, func(i, j int) bool { return i > j })
	return result, nil
}

func validAddress(value string) bool {
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, " \t\r\n@/\\") || strings.HasPrefix(value, "-") {
		return false
	}
	return net.ParseIP(value) != nil || hostnamePattern.MatchString(value)
}
func boundedDuration(raw string, fallback, min, max time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < min || d > max {
		return 0, errors.New("duration is outside the supported bound")
	}
	return d, nil
}
func validateCredentialFile(path, label string) error {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, " \t\r\n#\\\"'") {
		return fmt.Errorf("%s path must be absolute", label)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("%s must be a private regular non-symlink file", label)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%s must be owned by the current user", label)
	}
	limit := int64(64 << 10)
	if label == "known_hosts" {
		limit = 4 << 20
	}
	if info.Size() > limit {
		return fmt.Errorf("%s exceeds size limit", label)
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s parent directory is unsafe", label)
	}
	return nil
}
func readPrivateFile(path string, limit int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, errors.New("file must be private, regular and non-symlink")
	}
	if info.Size() > limit {
		return nil, errors.New("file exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds size limit")
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(info, after) || after.Size() != int64(len(data)) || after.ModTime() != info.ModTime() {
		return nil, errors.New("file changed while being read")
	}
	return data, nil
}
