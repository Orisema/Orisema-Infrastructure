// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type FileOperation struct {
	BackupID     string `json:"backupId"`
	Source       string `json:"source,omitempty"`
	SourceSHA256 string `json:"sourceSha256,omitempty"`
	Artifact     string `json:"artifact"`
	Destination  string `json:"destination,omitempty"`
	Staging      string `json:"staging"`
	PendingStep  string `json:"pendingStep,omitempty"`
}

type FileOperationRequest struct {
	ID, Type, Application, BackupID, Source, SourceSHA256, Artifact, Destination, Staging, FirstStep string
	At                                                                                               time.Time
}

var fileSequences = map[string][]string{
	"BACKUP":  {"stage", "payload", "metadata", "publish", "verify"},
	"RESTORE": {"stage", "payload", "publish", "verify"},
}

func (s *Store) CreateFileOperation(r FileOperationRequest) (Operation, error) {
	if _, ok := fileSequences[r.Type]; !ok || r.BackupID == "" || r.Artifact == "" || r.Staging == "" || r.FirstStep != "stage" {
		return Operation{}, errors.New("file operation request is invalid")
	}
	at := r.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	op := Operation{ID: r.ID, Application: r.Application, Services: []string{"configuration"}, Type: r.Type, State: Preparing, CreatedAt: at,
		FileOperation: &FileOperation{BackupID: r.BackupID, Source: r.Source, SourceSHA256: r.SourceSHA256, Artifact: r.Artifact, Destination: r.Destination, Staging: r.Staging, PendingStep: r.FirstStep}}
	if err := s.Create(op); err != nil {
		return Operation{}, err
	}
	return s.Load(r.ID)
}

// RecordFileOperationStep atomically records a completed filesystem boundary
// and the next intended step. It never performs filesystem mutations itself.
func (s *Store) RecordFileOperationStep(id, completed, next string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.FileOperation == nil || op.State.Terminal() || op.FileOperation.PendingStep != completed {
			return errors.New("file operation step is stale or out of order")
		}
		seq := fileSequences[op.Type]
		index := -1
		for i, step := range seq {
			if step == completed {
				index = i
				break
			}
		}
		wantNext := ""
		if index >= 0 && index+1 < len(seq) {
			wantNext = seq[index+1]
		}
		if index < 0 || next != wantNext {
			return errors.New("file operation next step is invalid")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("file operation timestamp regressed")
		}
		from, to := op.State, op.State
		switch completed {
		case "stage":
			to = Applying
		case "publish":
			to = Verifying
		case "verify":
			to = Completed
		}
		op.FileOperation.PendingStep = next
		op.State, op.UpdatedAt = to, at
		op.Events = append(op.Events, Event{At: at, From: from, To: to, Step: completed + "-completed", Detail: fmt.Sprintf("next=%s", next)})
		if err := validate(op); err != nil {
			return err
		}
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

func validateFileOperation(op Operation) error {
	f := op.FileOperation
	if f == nil {
		return nil
	}
	seq, ok := fileSequences[op.Type]
	if !ok || f.BackupID == "" || f.Artifact == "" || f.Staging == "" {
		return errors.New("persisted file operation is invalid")
	}
	expectedStaging := f.Artifact + ".staging-" + strings.TrimPrefix(op.ID, "backup-op-")
	if op.Type == "BACKUP" {
		hash, err := hex.DecodeString(f.SourceSHA256)
		if f.Source == "" || f.Destination != "" || err != nil || len(hash) != 32 || f.SourceSHA256 != strings.ToLower(f.SourceSHA256) || f.Staging != expectedStaging {
			return errors.New("persisted backup source or staging boundary is invalid")
		}
	} else {
		expectedStaging = f.Destination + ".staging-" + strings.TrimPrefix(op.ID, "restore-op-")
		if f.Source != f.Artifact || f.SourceSHA256 != "" || f.Destination == "" || f.Staging != expectedStaging {
			return errors.New("persisted restore source or staging boundary is invalid")
		}
	}
	valid := f.PendingStep == ""
	for _, step := range seq {
		valid = valid || f.PendingStep == step
	}
	if !valid {
		return errors.New("persisted file operation pending step is invalid")
	}
	if op.State == Completed && f.PendingStep != "" {
		return errors.New("completed file operation retains a pending step")
	}
	if op.State != Preparing && op.State != Applying && op.State != Verifying && op.State != Completed {
		return errors.New("persisted file operation state is invalid")
	}
	return nil
}
