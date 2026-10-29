// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

type HealthVerification struct {
	StartedAt    time.Time           `json:"startedAt"`
	Policy       HealthPolicy        `json:"policy"`
	Observations []HealthObservation `json:"observations,omitempty"`
	Evaluation   HealthEvaluation    `json:"evaluation"`
}

// StartHealthVerification atomically records intent and moves APPLYING to
// VERIFYING. It does not inspect a runtime or execute a probe.
func (s *Store) StartHealthVerification(id string, policy HealthPolicy, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.Health != nil {
			return errors.New("health verification has already been started")
		}
		if err := ValidateTransition(op.State, Verifying); err != nil {
			return err
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("health verification timestamp precedes current operation state")
		}
		evaluation, err := EvaluateHealth(at, at, policy, nil)
		if err != nil {
			return err
		}
		op.Events = append(op.Events, Event{At: at, From: op.State, To: Verifying, Step: "health-verification", Detail: "verification policy persisted"})
		op.State, op.UpdatedAt = Verifying, at
		op.Health = &HealthVerification{StartedAt: at, Policy: policy, Evaluation: evaluation}
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

// RecordHealthObservation durably appends one supplied observation and its
// deterministic evaluation. It does not collect the observation itself.
func (s *Store) RecordHealthObservation(id string, observation HealthObservation, now time.Time) (Operation, error) {
	return s.updateHealthVerification(id, &observation, now)
}

// RefreshHealthVerification persists a time-based reevaluation, allowing a
// timeout to be recorded without inventing an observation.
func (s *Store) RefreshHealthVerification(id string, now time.Time) (Operation, error) {
	return s.updateHealthVerification(id, nil, now)
}

func (s *Store) updateHealthVerification(id string, observation *HealthObservation, now time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Verifying || op.Health == nil {
			return errors.New("operation is not in a persisted VERIFYING state")
		}
		if op.Health.Evaluation.Terminal {
			return errors.New("health verification already has a terminal evaluation")
		}
		if now.IsZero() {
			now = time.Now().UTC()
		}
		now = now.UTC()
		if now.Before(op.UpdatedAt) {
			return errors.New("health evaluation timestamp precedes current operation state")
		}
		observations := append([]HealthObservation(nil), op.Health.Observations...)
		if observation != nil {
			observation.At = observation.At.UTC()
			observations = append(observations, *observation)
		}
		evaluation, err := EvaluateHealth(op.Health.StartedAt, now, op.Health.Policy, observations)
		if err != nil {
			return err
		}
		op.Health.Observations = observations
		op.Health.Evaluation = evaluation
		op.UpdatedAt = now
		detail := fmt.Sprintf("health evaluation %s", evaluation.Status)
		op.Events = append(op.Events, Event{At: now, From: Verifying, To: Verifying, Step: "health-observation", Detail: detail})
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

func validateHealthVerification(op Operation) error {
	if op.Health == nil {
		return nil
	}
	switch op.State {
	case Verifying, Completed, Failed, Interrupted, RecoveryRequired, RollingBack, RolledBack:
	default:
		return errors.New("health verification is present before the operation reached VERIFYING")
	}
	reachedVerifying := false
	for _, event := range op.Events {
		if event.To == Verifying {
			reachedVerifying = true
			break
		}
	}
	if !reachedVerifying {
		return errors.New("health verification has no VERIFYING transition event")
	}
	if op.Health.StartedAt.IsZero() || op.Health.StartedAt.Before(op.CreatedAt) {
		return errors.New("health verification start is invalid")
	}
	if op.Health.Evaluation.EvaluatedAt.IsZero() || op.Health.Evaluation.EvaluatedAt.Before(op.Health.StartedAt) {
		return errors.New("health verification evaluation timestamp is invalid")
	}
	recomputed, err := EvaluateHealth(op.Health.StartedAt, op.Health.Evaluation.EvaluatedAt, op.Health.Policy, op.Health.Observations)
	if err != nil {
		return fmt.Errorf("invalid persisted health verification: %w", err)
	}
	if !reflect.DeepEqual(recomputed, op.Health.Evaluation) {
		return errors.New("persisted health evaluation does not match its policy and observations")
	}
	return nil
}
