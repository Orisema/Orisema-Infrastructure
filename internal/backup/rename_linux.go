//go:build linux

// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

func renameNoReplace(from, to string) error {
	oldp, err := syscall.BytePtrFromString(from)
	if err != nil {
		return err
	}
	newp, err := syscall.BytePtrFromString(to)
	if err != nil {
		return err
	}
	var number uintptr
	switch runtime.GOARCH {
	case "amd64":
		number = 316
	case "arm64", "riscv64", "loong64":
		number = 276
	case "s390x":
		number = 347
	case "ppc64", "ppc64le":
		number = 357
	default:
		return fmt.Errorf("atomic no-replace publication is unsupported on %s", runtime.GOARCH)
	}
	const atFDCWD = ^uintptr(99)
	_, _, errno := syscall.Syscall6(number, atFDCWD, uintptr(unsafe.Pointer(oldp)), atFDCWD, uintptr(unsafe.Pointer(newp)), 1, 0)
	if errno != 0 {
		return fmt.Errorf("rename without replacement: %w", errno)
	}
	return nil
}
