//go:build !linux

// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import "os/exec"

func configureCancellation(command *exec.Cmd) {}
