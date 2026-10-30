// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Outcome string

const (
	OutcomeOK             Outcome = "OK"
	OutcomeAuthentication Outcome = "AUTHENTICATION_FAILED"
	OutcomeHostKey        Outcome = "HOST_KEY_FAILED"
	OutcomeTimeout        Outcome = "TIMEOUT"
	OutcomeConnection     Outcome = "CONNECTION_FAILED"
	OutcomeExecution      Outcome = "EXECUTION_FAILED"
)

type Result struct {
	Host     string
	Outcome  Outcome
	Duration time.Duration
}
type Transport struct{ Binary string }

func (t Transport) Check(ctx context.Context, registry Registry, name string) (Result, error) {
	if err := registry.Validate(); err != nil {
		return Result{}, fmt.Errorf("host registry changed or became unsafe: %w", err)
	}
	chain, err := registry.chain(name)
	if err != nil {
		return Result{}, err
	}
	target, _ := registry.Host(name)
	timeout, _ := time.ParseDuration(target.CommandTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "orisema-ssh-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		return Result{}, err
	}
	config := filepath.Join(dir, "config")
	if err := writeConfig(config, chain, name); err != nil {
		return Result{}, err
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
	args := []string{"-F", config, "-o", "BatchMode=yes", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectionAttempts=1", "-o", "ConnectTimeout=" + strconv.Itoa(seconds), "-o", "LogLevel=ERROR", name, "--", "true"}
	started := time.Now()
	command := exec.CommandContext(ctx, binary, args...)
	configureCancellation(command)
	command.WaitDelay = 2 * time.Second
	command.Stdin = nil
	var stderr bytes.Buffer
	command.Stdout = ioDiscard{}
	command.Stderr = &limitedWriter{target: &stderr, remaining: 8192}
	err = command.Run()
	result := Result{Host: name, Duration: time.Since(started)}
	if err == nil {
		result.Outcome = OutcomeOK
		return result, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Outcome = OutcomeTimeout
		return result, fmt.Errorf("SSH connectivity timed out")
	}
	result.Outcome = classify(stderr.String())
	return result, fmt.Errorf("SSH connectivity failed: %s", result.Outcome)
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
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	original := len(p)
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	if len(p) > 0 {
		_, _ = w.target.Write(p)
		w.remaining -= len(p)
	}
	return original, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
