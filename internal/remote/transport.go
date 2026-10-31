// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

type Outcome string

const (
	OutcomeOK             Outcome = "OK"
	OutcomeAuthentication Outcome = "AUTHENTICATION_FAILED"
	OutcomeHostKey        Outcome = "HOST_KEY_FAILED"
	OutcomeTimeout        Outcome = "TIMEOUT"
	OutcomeConnection     Outcome = "CONNECTION_FAILED"
	OutcomeExecution      Outcome = "EXECUTION_FAILED"
	inspectOutputLimit            = 2 << 20
	logsOutputLimit               = 1 << 20
)

type Result struct {
	Host     string
	Outcome  Outcome
	Duration time.Duration
}
type Transport struct{ Binary string }

func (t Transport) Check(ctx context.Context, registry Registry, name string) (Result, error) {
	_, result, err := t.execute(ctx, registry, name, []string{"true"}, 0)
	return result, err
}

// Inspect performs the only allowlisted metadata query. The container name is
// derived from validated identities rather than accepted as a command argument.
func (t Transport) Inspect(ctx context.Context, registry Registry, host, app, service string) (runtime.Inspection, Result, error) {
	name, err := safeContainerName(app, service)
	if err != nil {
		return runtime.Inspection{}, Result{}, err
	}
	out, result, err := t.execute(ctx, registry, host, []string{"podman", "inspect", "--type", "container", name}, inspectOutputLimit)
	if err != nil {
		return runtime.Inspection{}, result, err
	}
	var items []runtime.Inspection
	if err := json.Unmarshal(out, &items); err != nil || len(items) != 1 || items[0].ID == "" || items[0].Name != name {
		result.Outcome = OutcomeExecution
		return runtime.Inspection{}, result, errors.New("remote inspection returned invalid metadata")
	}
	if !runtime.Owned(items[0], app, service) {
		result.Outcome = OutcomeExecution
		return runtime.Inspection{}, result, errors.New("remote container ownership is not verified")
	}
	return items[0], result, nil
}

// Logs revalidates ownership immediately before a fixed 100-line log tail.
func (t Transport) Logs(ctx context.Context, registry Registry, host, app, service string) ([]byte, Result, error) {
	if _, result, err := t.Inspect(ctx, registry, host, app, service); err != nil {
		return nil, result, err
	}
	name, _ := safeContainerName(app, service)
	return t.execute(ctx, registry, host, []string{"podman", "logs", "--tail", "100", name}, logsOutputLimit)
}

func safeContainerName(app, service string) (string, error) {
	if !aliasPattern.MatchString(app) || !aliasPattern.MatchString(service) {
		return "", errors.New("application or service identity is invalid")
	}
	return "orisema-" + app + "-" + service, nil
}

func (t Transport) execute(ctx context.Context, registry Registry, name string, remoteArgs []string, outputLimit int) ([]byte, Result, error) {
	if err := registry.Validate(); err != nil {
		return nil, Result{}, fmt.Errorf("host registry changed or became unsafe: %w", err)
	}
	chain, err := registry.chain(name)
	if err != nil {
		return nil, Result{}, err
	}
	target, _ := registry.Host(name)
	timeout, _ := time.ParseDuration(target.CommandTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "orisema-ssh-")
	if err != nil {
		return nil, Result{}, err
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, Result{}, err
	}
	config := filepath.Join(dir, "config")
	if err := writeConfig(config, chain, name); err != nil {
		return nil, Result{}, err
	}
	binary := t.Binary
	if binary == "" {
		binary = "ssh"
	}
	connect, _ := time.ParseDuration(target.ConnectTimeout)
	seconds := int(connect / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	args := []string{"-F", config, "-o", "BatchMode=yes", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectionAttempts=1", "-o", "ConnectTimeout=" + strconv.Itoa(seconds), "-o", "LogLevel=ERROR", name, "--"}
	args = append(args, remoteArgs...)
	started := time.Now()
	command := exec.CommandContext(ctx, binary, args...)
	configureCancellation(command)
	command.WaitDelay = 2 * time.Second
	command.Stdin = nil
	var stdout, stderr bytes.Buffer
	limitedOut := &limitedWriter{target: &stdout, remaining: outputLimit}
	if outputLimit == 0 {
		command.Stdout = ioDiscard{}
	} else {
		command.Stdout = limitedOut
	}
	command.Stderr = &limitedWriter{target: &stderr, remaining: 8192}
	err = command.Run()
	result := Result{Host: name, Duration: time.Since(started)}
	if err == nil {
		if limitedOut.exceeded {
			result.Outcome = OutcomeExecution
			return nil, result, errors.New("remote output exceeded limit")
		}
		result.Outcome = OutcomeOK
		return stdout.Bytes(), result, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Outcome = OutcomeTimeout
		return nil, result, errors.New("SSH connectivity timed out")
	}
	result.Outcome = classify(stderr.String())
	return nil, result, fmt.Errorf("SSH connectivity failed: %s", result.Outcome)
}

func writeConfig(path string, chain []Host, target string) error {
	var b strings.Builder
	for _, h := range chain {
		fmt.Fprintf(&b, "Host %s\n  HostName %s\n  Port %d\n  User %s\n  IdentityFile %s\n  IdentitiesOnly yes\n  UserKnownHostsFile %s\n  StrictHostKeyChecking yes\n  BatchMode yes\n  PasswordAuthentication no\n  KbdInteractiveAuthentication no\n", h.Name, h.Address, h.Port, h.User, h.IdentityFile, h.KnownHostsFile)
		if h.Name == target && h.ProxyJump != "" {
			fmt.Fprintf(&b, "  ProxyJump %s\n", h.ProxyJump)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func classify(message string) Outcome {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "host key verification failed") || strings.Contains(lower, "remote host identification has changed"):
		return OutcomeHostKey
	case strings.Contains(lower, "permission denied") || strings.Contains(lower, "authentication failed"):
		return OutcomeAuthentication
	case strings.Contains(lower, "timed out"):
		return OutcomeTimeout
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no route to host") || strings.Contains(lower, "could not resolve hostname"):
		return OutcomeConnection
	default:
		return OutcomeExecution
	}
}

type limitedWriter struct {
	target    *bytes.Buffer
	remaining int
	exceeded  bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > w.remaining {
		p = p[:w.remaining]
		w.exceeded = true
	}
	if len(p) > 0 {
		_, _ = w.target.Write(p)
		w.remaining -= len(p)
	}
	return n, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
