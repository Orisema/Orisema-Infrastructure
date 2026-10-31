// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeSSH(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	script := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$*\" > \"$ORISEMA_SSH_ARGS\"\ncp \"$2\" \"$ORISEMA_SSH_CONFIG\"\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTransportUsesFixedStrictArgumentsAndGeneratedProxyConfig(t *testing.T) {
	registryPath, _, _ := registryFixture(t, `apiVersion: orisema.dev/hosts/v1alpha1
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
`)
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	argsPath, configPath := filepath.Join(root, "args"), filepath.Join(root, "config")
	t.Setenv("ORISEMA_SSH_ARGS", argsPath)
	t.Setenv("ORISEMA_SSH_CONFIG", configPath)
	binary := fakeSSH(t, "exit 0")
	result, err := (Transport{Binary: binary}).Check(context.Background(), *registry, "private")
	if err != nil || result.Outcome != OutcomeOK {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	args, _ := os.ReadFile(argsPath)
	text := string(args)
	for _, want := range []string{"BatchMode=yes", "PasswordAuthentication=no", "StrictHostKeyChecking=yes", "private -- true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("args=%q missing %q", text, want)
		}
	}
	config, _ := os.ReadFile(configPath)
	cfg := string(config)
	for _, want := range []string{"Host bastion", "Host private", "ProxyJump bastion", "IdentitiesOnly yes", "UserKnownHostsFile"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("config missing %q: %s", want, cfg)
		}
	}
}

func TestTransportClassifiesFailuresWithoutReturningSSHText(t *testing.T) {
	registryPath, _, _ := registryFixture(t, directRegistry)
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, message string
		want          Outcome
	}{{"host-key", "Host key verification failed.", OutcomeHostKey}, {"auth", "Permission denied (publickey).", OutcomeAuthentication}, {"connection", "Connection refused", OutcomeConnection}, {"other", "unexpected remote output SECRET", OutcomeExecution}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("ORISEMA_SSH_ARGS", filepath.Join(root, "args"))
			t.Setenv("ORISEMA_SSH_CONFIG", filepath.Join(root, "config"))
			binary := fakeSSH(t, "echo '"+tc.message+"' >&2\nexit 255")
			result, err := (Transport{Binary: binary}).Check(context.Background(), *registry, "primary")
			if err == nil || result.Outcome != tc.want {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatal("SSH stderr leaked")
			}
		})
	}
}

func TestTransportTimeoutCancelsProcess(t *testing.T) {
	content := strings.Replace(directRegistry, "commandTimeout: 3s", "commandTimeout: 1s", 1)
	registryPath, _, _ := registryFixture(t, content)
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("ORISEMA_SSH_ARGS", filepath.Join(root, "args"))
	t.Setenv("ORISEMA_SSH_CONFIG", filepath.Join(root, "config"))
	binary := fakeSSH(t, "sleep 5")
	started := time.Now()
	result, err := (Transport{Binary: binary}).Check(context.Background(), *registry, "primary")
	if err == nil || result.Outcome != OutcomeTimeout || time.Since(started) > 3*time.Second {
		t.Fatalf("result=%#v elapsed=%s err=%v", result, time.Since(started), err)
	}
}

func TestRemoteInspectAndLogsAreFixedBoundedAndOwnershipChecked(t *testing.T) {
	registryPath, _, _ := registryFixture(t, directRegistry)
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("ORISEMA_SSH_ARGS", filepath.Join(root, "args"))
	t.Setenv("ORISEMA_SSH_CONFIG", filepath.Join(root, "config"))
	valid := `[{"Id":"abc","Name":"orisema-demo-web","State":{"Status":"running","Running":true},"Config":{"Image":"nginx:stable","Labels":{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web"}}}]`
	binary := fakeSSH(t, `
case "$*" in
  *"podman inspect --type container orisema-demo-web") printf '%s\n' '`+valid+`' ;;
  *"podman logs --tail 100 orisema-demo-web") printf 'safe log\n' ;;
  *) exit 90 ;;
esac`)
	item, result, err := (Transport{Binary: binary}).Inspect(context.Background(), *registry, "primary", "demo", "web")
	if err != nil || result.Outcome != OutcomeOK || item.ID != "abc" {
		t.Fatalf("item=%#v result=%#v err=%v", item, result, err)
	}
	out, result, err := (Transport{Binary: binary}).Logs(context.Background(), *registry, "primary", "demo", "web")
	if err != nil || string(out) != "safe log\n" || result.Outcome != OutcomeOK {
		t.Fatalf("out=%q result=%#v err=%v", out, result, err)
	}
	args, _ := os.ReadFile(filepath.Join(root, "args"))
	if !strings.Contains(string(args), "podman logs --tail 100 orisema-demo-web") {
		t.Fatalf("unexpected args %q", args)
	}
	if _, _, err := (Transport{Binary: binary}).Inspect(context.Background(), *registry, "primary", "demo;touch", "web"); err == nil {
		t.Fatal("unsafe identity accepted")
	}
}

func TestRemoteInspectRejectsWrongOwnershipAndOversizedOutput(t *testing.T) {
	registryPath, _, _ := registryFixture(t, directRegistry)
	registry, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, body string }{
		{"ownership", `printf '%s\n' '[{"Id":"abc","Name":"orisema-demo-web","Config":{"Labels":{"dev.orisema.managed":"true","dev.orisema.app":"other","dev.orisema.service":"web"}}}]'`},
		{"oversized", `dd if=/dev/zero bs=1048576 count=3 2>/dev/null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("ORISEMA_SSH_ARGS", filepath.Join(root, "args"))
			t.Setenv("ORISEMA_SSH_CONFIG", filepath.Join(root, "config"))
			_, result, err := (Transport{Binary: fakeSSH(t, tc.body)}).Inspect(context.Background(), *registry, "primary", "demo", "web")
			if err == nil || result.Outcome != OutcomeExecution {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}
