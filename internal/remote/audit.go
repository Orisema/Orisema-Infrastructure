// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type AuditRecord struct {
	Version     string  `json:"version"`
	Time        string  `json:"time"`
	RequestID   string  `json:"request_id"`
	Phase       string  `json:"phase"`
	Host        string  `json:"host"`
	Action      string  `json:"action"`
	Application string  `json:"application"`
	Service     string  `json:"service,omitempty"`
	Outcome     Outcome `json:"outcome,omitempty"`
	DurationMS  int64   `json:"duration_ms,omitempty"`
}

func NewRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate audit request ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

// AppendAudit durably appends one bounded JSON record to a private local file.
func AppendAudit(path string, record AuditRecord) error {
	if !filepath.IsAbs(path) {
		return errors.New("audit path must be absolute")
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&0o022 != 0 {
		return errors.New("audit parent must be a private non-symlink directory")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("open audit: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("audit must be a private regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("audit must be owned by the current user")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock audit: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	record.Version = "orisema.remote.audit/v1"
	record.Time = time.Now().UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(record)
	if err != nil || len(encoded) > 4096 {
		return errors.New("encode audit record")
	}
	encoded = append(encoded, '\n')
	if _, err := f.Write(encoded); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync audit: %w", err)
	}
	return nil
}
