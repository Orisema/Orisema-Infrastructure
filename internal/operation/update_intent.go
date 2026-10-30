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

var updateComparedFields = []string{"ownership", "container-id", "ports", "healthcheck", "restart-policy", "configured-image"}

const UpdateAuthorizationTTL = 5 * time.Minute

type UpdateIntent struct {
	Service         string    `json:"service"`
	CurrentImage    string    `json:"currentImage"`
	TargetImage     string    `json:"targetImage"`
	ComparedFields  []string  `json:"comparedFields"`
	PlanFingerprint string    `json:"planFingerprint"`
	PlannedAt       time.Time `json:"plannedAt"`
}

type UpdateIntentRequest struct {
	ID           string
	Application  string
	Service      string
	ResourceName string
	ResourceID   string
	CurrentImage string
	TargetImage  string
	At           time.Time
}

type UpdateAuthorization struct {
	At                time.Time        `json:"at"`
	PlanFingerprint   string           `json:"planFingerprint"`
	ObservedResources []Resource       `json:"observedResources"`
	ResourceEvidence  ResourceEvidence `json:"resourceEvidence"`
}

func (op Operation) IntentResourceName() string {
	if len(op.Resources) == 0 {
		return ""
	}
	return op.Resources[0].Name
}

func UpdatePlanFingerprint(application, service, resourceName, resourceID, currentImage, targetImage string) string {
	payload := struct {
		Contract       string   `json:"contract"`
		Application    string   `json:"application"`
		Service        string   `json:"service"`
		ResourceName   string   `json:"resourceName"`
		ResourceID     string   `json:"resourceId"`
		CurrentImage   string   `json:"currentImage"`
		TargetImage    string   `json:"targetImage"`
		ComparedFields []string `json:"comparedFields"`
	}{"orisema-update-intent-v1", application, service, resourceName, resourceID, currentImage, targetImage, updateComparedFields}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// CreateUpdateIntent persists a plan-bound PENDING operation. Runtime evidence
// is supplied by the caller; this method has no runtime access or mutation.
func (s *Store) CreateUpdateIntent(request UpdateIntentRequest) (Operation, error) {
	at := request.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	at = at.UTC()
	intent := &UpdateIntent{
		Service: request.Service, CurrentImage: request.CurrentImage, TargetImage: request.TargetImage,
		ComparedFields: append([]string(nil), updateComparedFields...), PlannedAt: at,
		PlanFingerprint: UpdatePlanFingerprint(request.Application, request.Service, request.ResourceName, request.ResourceID, request.CurrentImage, request.TargetImage),
	}
	op := Operation{
		ID: request.ID, Application: request.Application, Services: []string{request.Service}, Type: "UPDATE", CreatedAt: at,
		Resources: []Resource{{Kind: "container", Name: request.ResourceName, ID: request.ResourceID, Ownership: OwnershipManaged, Application: request.Application, Service: request.Service}},
		Previous:  request.CurrentImage, RecoveryData: request.ResourceID, Intent: intent,
	}
	if err := s.Create(op); err != nil {
		return Operation{}, err
	}
	return s.Load(request.ID)
}

// AuthorizeUpdateIntent atomically advances a freshly revalidated intent to
// PREPARING. Confirmation is a CLI concern; this method only accepts exact
// plan and resource evidence and never accesses or mutates a runtime.
func (s *Store) AuthorizeUpdateIntent(id, planFingerprint string, observedResources []Resource, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Pending || op.Intent == nil || op.Authorization != nil {
			return errors.New("update intent is not pending authorization")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("update authorization timestamp precedes the intent")
		}
		if at.After(op.Intent.PlannedAt.Add(UpdateAuthorizationTTL)) {
			return errors.New("update intent authorization window has expired")
		}
		if planFingerprint != op.Intent.PlanFingerprint {
			return errors.New("update plan fingerprint changed before authorization")
		}
		evidence := RevalidateResources(op.Resources, observedResources)
		if !evidence.Safe {
			reason := "fresh resource evidence does not match the update intent"
			if len(evidence.Issues) != 0 {
				reason += ": " + evidence.Issues[0]
			}
			return errors.New(reason)
		}
		observed := append([]Resource(nil), observedResources...)
		op.Authorization = &UpdateAuthorization{At: at, PlanFingerprint: planFingerprint, ObservedResources: observed, ResourceEvidence: evidence}
		op.Events = append(op.Events, Event{At: at, From: Pending, To: Preparing, Step: "update-authorized", Detail: "plan and resource evidence revalidated"})
		op.State, op.UpdatedAt = Preparing, at
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

func validateUpdateIntent(op Operation) error {
	if op.Intent == nil {
		return nil
	}
	intent := op.Intent
	if op.Type != "UPDATE" || intent.Service == "" || intent.CurrentImage == "" || intent.TargetImage == "" || intent.CurrentImage == intent.TargetImage {
		return errors.New("persisted update intent fields are invalid")
	}
	if intent.PlannedAt.IsZero() || intent.PlannedAt != op.CreatedAt || !reflect.DeepEqual(intent.ComparedFields, updateComparedFields) {
		return errors.New("persisted update intent contract is invalid")
	}
	var resource *Resource
	for i := range op.Resources {
		candidate := &op.Resources[i]
		if candidate.Kind == "container" && candidate.Service == intent.Service {
			if resource != nil {
				return errors.New("persisted update intent has ambiguous container evidence")
			}
			resource = candidate
		}
	}
	if resource == nil || resource.Ownership != OwnershipManaged || resource.ID == "" || resource.Application != op.Application {
		return errors.New("persisted update intent lacks exact managed resource evidence")
	}
	want := UpdatePlanFingerprint(op.Application, intent.Service, resource.Name, resource.ID, intent.CurrentImage, intent.TargetImage)
	if intent.PlanFingerprint != want {
		return fmt.Errorf("persisted update intent fingerprint mismatch")
	}
	if op.Previous != intent.CurrentImage || op.RecoveryData != resource.ID {
		return errors.New("persisted update intent recovery evidence is inconsistent")
	}
	return nil
}

func validateUpdateAuthorization(op Operation) error {
	if op.Authorization == nil {
		return nil
	}
	if op.Intent == nil || op.State == Pending {
		return errors.New("persisted update authorization has no advanced intent")
	}
	authorization := op.Authorization
	if authorization.At.Before(op.Intent.PlannedAt) || authorization.At.After(op.Intent.PlannedAt.Add(UpdateAuthorizationTTL)) || authorization.PlanFingerprint != op.Intent.PlanFingerprint {
		return errors.New("persisted update authorization time or plan is invalid")
	}
	evidence := RevalidateResources(op.Resources, authorization.ObservedResources)
	if !evidence.Safe || !reflect.DeepEqual(evidence, authorization.ResourceEvidence) {
		return errors.New("persisted update authorization does not match resource evidence")
	}
	found := false
	for _, event := range op.Events {
		if event.At == authorization.At && event.From == Pending && event.To == Preparing && event.Step == "update-authorized" {
			found = true
			break
		}
	}
	if !found {
		return errors.New("persisted update authorization has no matching transition event")
	}
	return nil
}
