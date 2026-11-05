// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

const DeployContract = "local-deploy-v1"

type DeployTarget struct {
	Service             string `json:"service"`
	ResourceName        string `json:"resourceName"`
	Image               string `json:"image"`
	BindingsFingerprint string `json:"bindingsFingerprint"`
}

type DeployResult struct {
	Service      string `json:"service"`
	ResourceName string `json:"resourceName"`
	ResourceID   string `json:"resourceId"`
}

type DeployIntent struct {
	Contract        string         `json:"contract"`
	PlanFingerprint string         `json:"planFingerprint"`
	Targets         []DeployTarget `json:"targets"`
	PendingService  string         `json:"pendingService,omitempty"`
	Completed       []DeployResult `json:"completed,omitempty"`
}

func DeployPlanFingerprint(application string, targets []DeployTarget) string {
	data, _ := json.Marshal(struct {
		Contract    string         `json:"contract"`
		Application string         `json:"application"`
		Targets     []DeployTarget `json:"targets"`
	}{DeployContract, application, targets})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Store) PrepareDeployStep(id, service string, at time.Time) (Operation, error) {
	return s.updateDeploy(id, at, func(op *Operation) error {
		d := op.Deploy
		if op.State != Applying || d.PendingService != "" || len(d.Completed) >= len(d.Targets) || d.Targets[len(d.Completed)].Service != service {
			return errors.New("deploy step is stale or out of order")
		}
		d.PendingService = service
		op.Events = append(op.Events, Event{At: at.UTC(), From: op.State, To: op.State, Step: "create-prepared", Detail: "service=" + service})
		return nil
	})
}

func (s *Store) RecordDeployStep(id, service, resourceID string, at time.Time) (Operation, error) {
	return s.updateDeploy(id, at, func(op *Operation) error {
		d := op.Deploy
		index := len(d.Completed)
		if op.State != Applying || resourceID == "" || index >= len(d.Targets) || d.PendingService != service || d.Targets[index].Service != service {
			return errors.New("deploy result is stale or out of order")
		}
		target := d.Targets[index]
		d.Completed = append(d.Completed, DeployResult{Service: service, ResourceName: target.ResourceName, ResourceID: resourceID})
		d.PendingService = ""
		op.Resources = append(op.Resources, Resource{Kind: "container", Name: target.ResourceName, ID: resourceID, Ownership: OwnershipManaged, Application: op.Application, Service: service})
		op.Events = append(op.Events, Event{At: at.UTC(), From: op.State, To: op.State, Step: "create-recorded", Detail: "service=" + service})
		return nil
	})
}

func (s *Store) updateDeploy(id string, at time.Time, update func(*Operation) error) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.Deploy == nil {
			return errors.New("operation is not a deploy")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("deploy event timestamp regressed")
		}
		if err := update(&op); err != nil {
			return err
		}
		op.UpdatedAt = at
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

func validateDeployIntent(op Operation) error {
	if op.Deploy == nil {
		return nil
	}
	d := op.Deploy
	if op.Type != "DEPLOY" || d.Contract != DeployContract || len(d.Targets) == 0 || len(op.Services) != len(d.Targets) || d.PlanFingerprint != DeployPlanFingerprint(op.Application, d.Targets) {
		return errors.New("persisted deploy intent is invalid")
	}
	for i, target := range d.Targets {
		if target.Service == "" || target.ResourceName == "" || target.Image == "" || len(target.BindingsFingerprint) != 64 || op.Services[i] != target.Service {
			return errors.New("persisted deploy target is invalid")
		}
	}
	if len(d.Completed) > len(d.Targets) || len(op.Resources) != len(d.Completed) {
		return errors.New("persisted deploy progress is invalid")
	}
	for i, completed := range d.Completed {
		target := d.Targets[i]
		resource := op.Resources[i]
		if completed.Service != target.Service || completed.ResourceName != target.ResourceName || completed.ResourceID == "" || resource.Name != completed.ResourceName || resource.ID != completed.ResourceID || resource.Service != completed.Service {
			return fmt.Errorf("persisted deploy result %d does not match its target", i)
		}
	}
	if d.PendingService != "" && (len(d.Completed) >= len(d.Targets) || d.PendingService != d.Targets[len(d.Completed)].Service) {
		return errors.New("persisted deploy pending step is invalid")
	}
	if (op.State == Verifying || op.State == Completed) && (d.PendingService != "" || len(d.Completed) != len(d.Targets)) {
		return errors.New("deploy verification started before all results were recorded")
	}
	var wantResources []Resource
	if len(d.Completed) > 0 {
		wantResources = make([]Resource, len(d.Completed))
	}
	for i, completed := range d.Completed {
		wantResources[i] = Resource{Kind: "container", Name: completed.ResourceName, ID: completed.ResourceID, Ownership: OwnershipManaged, Application: op.Application, Service: completed.Service}
	}
	if !reflect.DeepEqual(op.Resources, wantResources) {
		return errors.New("persisted deploy resources do not match results")
	}
	return nil
}
