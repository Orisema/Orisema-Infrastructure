// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func registryFixture(t *testing.T, hosts string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	key := filepath.Join(root, "id_ed25519")
	known := filepath.Join(root, "known_hosts")
	if err := os.WriteFile(key, []byte("test-key-reference"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(known, []byte("example.invalid ssh-ed25519 AAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content := strings.ReplaceAll(strings.ReplaceAll(hosts, "KEY", key), "KNOWN", known)
	registry := filepath.Join(root, "hosts.yaml")
	if err := os.WriteFile(registry, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return registry, key, known
}

const directRegistry = `apiVersion: orisema.dev/hosts/v1alpha1
kind: HostRegistry
hosts:
  - name: primary
    address: example.invalid
    port: 22
    user: deploy
    identityFile: KEY
    knownHostsFile: KNOWN
    connectTimeout: 2s
    commandTimeout: 3s
`

func TestLoadRegistryValidatesPrivateFilesAndDefaults(t *testing.T) {
	path, _, _ := registryFixture(t, directRegistry)
	registry, err := LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	host, ok := registry.Host("primary")
	if !ok || host.ConnectTimeout != "2s" || host.CommandTimeout != "3s" {
		t.Fatalf("host=%#v", host)
	}
}

func TestRegistryRejectsUnsafeAndInjectionShapedInputs(t *testing.T) {
	tests := []struct{ name, old, new string }{{"address", "example.invalid", "example.invalid;id"}, {"user", "deploy", "root -oProxyCommand=id"}, {"alias", "primary", "-primary"}, {"newline", "example.invalid", "example.invalid\\nHost evil"}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, _, _ := registryFixture(t, strings.Replace(directRegistry, tc.old, tc.new, 1))
			if _, err := LoadRegistry(path); err == nil {
				t.Fatal("unsafe registry loaded")
			}
		})
	}
	t.Run("registry-mode", func(t *testing.T) {
		path, _, _ := registryFixture(t, directRegistry)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRegistry(path); err == nil {
			t.Fatal("unsafe registry mode loaded")
		}
	})
	t.Run("identity-symlink", func(t *testing.T) {
		path, key, _ := registryFixture(t, directRegistry)
		target := key + ".target"
		if err := os.Rename(key, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, key); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRegistry(path); err == nil {
			t.Fatal("symlink identity loaded")
		}
	})
}

func TestRegistryProxyJumpChainAndCycle(t *testing.T) {
	content := `apiVersion: orisema.dev/hosts/v1alpha1
kind: HostRegistry
hosts:
  - name: bastion
    address: 192.0.2.10
    port: 2222
    user: jump
    identityFile: KEY
    knownHostsFile: KNOWN
  - name: private
    address: 10.0.0.8
    port: 22
    user: deploy
    identityFile: KEY
    knownHostsFile: KNOWN
    proxyJump: bastion
`
	path, _, _ := registryFixture(t, content)
	registry, err := LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	chain, err := registry.chain("private")
	if err != nil || len(chain) != 2 || chain[0].Name != "bastion" || chain[1].Name != "private" {
		t.Fatalf("chain=%#v err=%v", chain, err)
	}
	cycle := strings.Replace(content, "    knownHostsFile: KNOWN\n  - name: private", "    knownHostsFile: KNOWN\n    proxyJump: private\n  - name: private", 1)
	cyclePath, _, _ := registryFixture(t, cycle)
	if _, err := LoadRegistry(cyclePath); err == nil {
		t.Fatal("proxy cycle loaded")
	}
}
