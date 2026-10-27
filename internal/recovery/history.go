// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
)

const (
	historyVersion = 1
	maxHistorySize = 1 << 20
)

// HistoryRecord is an operator-supplied audit entry. Recording an entry does
// not perform or prove a runtime mutation.
type HistoryRecord struct {
	Version   int               `json:"version"`
	Manifest  string            `json:"manifest"`
	Service   string            `json:"service"`
	Action    diagnostic.Action `json:"action"`
	Timestamp time.Time         `json:"timestamp"`
	Succeeded bool              `json:"succeeded"`
}

func LoadHistory(path, manifestName string) ([]Attempt, error) {
	f, err := openHistory(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, fmt.Errorf("lock recovery history: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect recovery history: %w", err)
	}
	if info.Size() > maxHistorySize {
		return nil, fmt.Errorf("recovery history exceeds %d bytes", maxHistorySize)
	}

	var attempts []Attempt
	scanner := bufio.NewScanner(io.LimitReader(f, maxHistorySize+1))
	scanner.Buffer(make([]byte, 4096), 64*1024)
	line := 0
	for scanner.Scan() {
		line++
		var record HistoryRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("recovery history line %d: %w", line, err)
		}
		if err := validateRecord(record); err != nil {
			return nil, fmt.Errorf("recovery history line %d: %w", line, err)
		}
		if record.Manifest == manifestName {
			attempts = append(attempts, Attempt{Service: record.Service, Action: record.Action, Timestamp: record.Timestamp, Succeeded: record.Succeeded})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read recovery history: %w", err)
	}
	return attempts, nil
}

func AppendHistory(path string, record HistoryRecord) error {
	record.Version = historyVersion
	if err := validateRecord(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode recovery history: %w", err)
	}
	encoded = append(encoded, '\n')

	f, err := openHistory(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock recovery history: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("inspect recovery history: %w", err)
	}
	if info.Size()+int64(len(encoded)) > maxHistorySize {
		return fmt.Errorf("recovery history would exceed %d bytes", maxHistorySize)
	}
	if _, err := f.Write(encoded); err != nil {
		return fmt.Errorf("append recovery history: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync recovery history: %w", err)
	}
	return nil
}

func openHistory(path string, flags int, perm os.FileMode) (*os.File, error) {
	if path == "" {
		return nil, errors.New("recovery history path is required")
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("recovery history must not be a symbolic link")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect recovery history path: %w", err)
	}
	f, err := os.OpenFile(path, flags|syscall.O_NOFOLLOW, perm)
	if err != nil {
		return nil, fmt.Errorf("open recovery history: %w", err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("recovery history must be a regular file")
	}
	if flags&(os.O_WRONLY|os.O_RDWR) != 0 {
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			return nil, fmt.Errorf("restrict recovery history permissions: %w", err)
		}
	} else if info.Mode().Perm()&0o077 != 0 {
		f.Close()
		return nil, errors.New("recovery history permissions must not grant group or other access")
	}
	return f, nil
}

func validateRecord(record HistoryRecord) error {
	if record.Version != historyVersion {
		return fmt.Errorf("unsupported history version %d", record.Version)
	}
	if record.Manifest == "" || record.Service == "" {
		return errors.New("manifest and service are required")
	}
	if record.Action != diagnostic.ActionStart {
		return errors.New("only START attempts can be recorded")
	}
	if record.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	return nil
}
