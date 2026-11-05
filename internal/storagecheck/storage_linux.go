// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

// Package storagecheck provides read-only filesystem capacity checks.
package storagecheck

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

const MinimumAvailable uint64 = 256 << 20

// Available returns bytes available to the current user. The final path must
// be a real directory rather than a symlink.
func Available(path string) (uint64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, fmt.Errorf("inspect storage path: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("storage path must be a real directory")
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("measure available storage: %w", err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func Sufficient(available uint64) bool { return available >= MinimumAvailable }
