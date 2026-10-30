//go:build !linux

// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package backup

import "errors"

func renameNoReplace(_, _ string) error {
	return errors.New("atomic no-replace publication requires Linux")
}
