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
