// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

const Version = 1

var ErrApplicationBusy = errors.New("application already has an incomplete operation")

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type Resource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	ID        string `json:"id,omitempty"`
	Ownership string `json:"ownership"`
}

type Event struct {
	At     time.Time `json:"at"`
	From   State     `json:"from,omitempty"`
	To     State     `json:"to"`
	Step   string    `json:"step,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

type Operation struct {
	Version      int        `json:"version"`
	ID           string     `json:"id"`
	Application  string     `json:"application"`
	Services     []string   `json:"services"`
	Type         string     `json:"type"`
	State        State      `json:"state"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Resources    []Resource `json:"resources,omitempty"`
	Previous     string     `json:"previous,omitempty"`
	Failure      string     `json:"failure,omitempty"`
	RecoveryData string     `json:"recoveryData,omitempty"`
	Events       []Event    `json:"events"`
}

type Store struct{ root string }

func OpenStore(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("operation journal directory is required")
	}
	if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create operation journal directory: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("operation journal path must be a real directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("operation journal directory permissions must not grant group or other access")
	}
	return &Store{root: root}, nil
}

func (s *Store) Create(op Operation) error {
	if op.Version == 0 {
		op.Version = Version
	}
	if op.State == "" {
		op.State = Pending
	}
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	}
	op.CreatedAt = op.CreatedAt.UTC()
	op.UpdatedAt = op.CreatedAt
	op.Events = []Event{{At: op.CreatedAt, To: op.State, Detail: "operation created"}}
	if err := validate(op); err != nil {
		return err
	}
	return s.withLock(".store.lock", func() error {
		path, err := s.path(op.ID)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(path); err == nil {
			return errors.New("operation already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		operations, err := s.listUnlocked()
		if err != nil {
			return fmt.Errorf("check concurrent operations: %w", err)
		}
		for _, existing := range operations {
			if existing.Application == op.Application && !existing.State.Terminal() {
				return fmt.Errorf("%w: %s (%s)", ErrApplicationBusy, existing.ID, existing.State)
			}
		}
		return s.atomicWrite(path, op)
	})
}

// List returns every validated operation in deterministic creation order. A
// single corrupt or unsafe snapshot makes the entire query fail closed.
func (s *Store) List() ([]Operation, error) {
	var result []Operation
	err := s.withLock(".store.lock", func() error {
		var err error
		result, err = s.listUnlocked()
		return err
	})
	return result, err
}

// Incomplete returns only non-terminal operations. It is read-only and does
// not infer that any operation can be resumed.
func (s *Store) Incomplete() ([]Operation, error) {
	operations, err := s.List()
	if err != nil {
		return nil, err
	}
	result := make([]Operation, 0, len(operations))
	for _, op := range operations {
		if !op.State.Terminal() {
			result = append(result, op)
		}
	}
	return result, nil
}

func (s *Store) listUnlocked() ([]Operation, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list operation journals: %w", err)
	}
	operations := make([]Operation, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !safeID.MatchString(id) {
			return nil, fmt.Errorf("unsafe operation journal filename %q", name)
		}
		op, err := s.Load(id)
		if err != nil {
			return nil, err
		}
		if op.ID != id {
			return nil, fmt.Errorf("operation journal filename %q does not match ID %q", name, op.ID)
		}
		operations = append(operations, op)
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].CreatedAt.Equal(operations[j].CreatedAt) {
			return operations[i].ID < operations[j].ID
		}
		return operations[i].CreatedAt.Before(operations[j].CreatedAt)
	})
	return operations, nil
}

func (s *Store) Load(id string) (Operation, error) {
	path, err := s.path(id)
	if err != nil {
		return Operation{}, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return Operation{}, fmt.Errorf("open operation journal: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Operation{}, fmt.Errorf("inspect operation journal: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return Operation{}, errors.New("operation journal file is unsafe")
	}
	var op Operation
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&op); err != nil {
		return Operation{}, fmt.Errorf("decode operation journal: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Operation{}, errors.New("decode operation journal: trailing JSON value")
		}
		return Operation{}, fmt.Errorf("decode operation journal: %w", err)
	}
	if err := validate(op); err != nil {
		return Operation{}, fmt.Errorf("invalid operation journal: %w", err)
	}
	return op, nil
}

func (s *Store) Transition(id string, to State, step, detail string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if err := ValidateTransition(op.State, to); err != nil {
			return err
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("operation event timestamp precedes current state")
		}
		step, detail = strings.TrimSpace(step), strings.TrimSpace(detail)
		if len(step) > 128 || len(detail) > 1024 {
			return errors.New("operation event step or detail is too long")
		}
		op.Events = append(op.Events, Event{At: at, From: op.State, To: to, Step: step, Detail: detail})
		op.State, op.UpdatedAt = to, at
		if to == Failed || to == RecoveryRequired {
			op.Failure = detail
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

type Assessment struct {
	Terminal          bool
	ResumeSafe        bool
	RollbackCandidate bool
	Intervention      bool
	Reason            string
}

func Assess(op Operation) Assessment {
	if op.State.Terminal() {
		return Assessment{Terminal: true, Reason: "operation reached a terminal state"}
	}
	a := Assessment{Intervention: true, Reason: "operation is incomplete; inspect journal and runtime evidence before any mutation"}
	if op.State == RecoveryRequired && op.Previous != "" && op.RecoveryData != "" {
		a.RollbackCandidate = true
		a.Reason = "rollback evidence is recorded but still requires ownership revalidation and explicit authorization"
	}
	return a
}

func (s *Store) path(id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", errors.New("operation ID must contain only safe ASCII identifier characters")
	}
	return filepath.Join(s.root, id+".json"), nil
}

func (s *Store) withLock(name string, fn func() error) error {
	f, err := os.OpenFile(filepath.Join(s.root, name), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("open operation journal lock: %w", err)
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock operation journal: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func (s *Store) atomicWrite(path string, op Operation) error {
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return fmt.Errorf("encode operation journal: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(s.root, ".operation-*.tmp")
	if err != nil {
		return fmt.Errorf("create operation journal temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write operation journal: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync operation journal: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit operation journal: %w", err)
	}
	dir, err := os.Open(s.root)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync operation journal directory: %w", err)
	}
	return nil
}

func validate(op Operation) error {
	if op.Version != Version {
		return fmt.Errorf("unsupported operation journal version %d", op.Version)
	}
	if !safeID.MatchString(op.ID) || op.Application == "" || op.Type == "" || len(op.Services) == 0 {
		return errors.New("operation ID, application, type and services are required")
	}
	if !ValidState(op.State) || op.CreatedAt.IsZero() || op.UpdatedAt.IsZero() || op.UpdatedAt.Before(op.CreatedAt) {
		return errors.New("operation state or timestamps are invalid")
	}
	if len(op.Events) == 0 || op.Events[len(op.Events)-1].To != op.State {
		return errors.New("operation events do not describe the current state")
	}
	return nil
}
