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
	"sort"
	"time"
)

var updateComparedFields = []string{"ownership", "container-id", "ports", "healthcheck", "restart-policy", "configured-image"}

const UpdateAuthorizationTTL = 5 * time.Minute

const updateIntentV2 = "orisema-update-intent-v2"
const updateIntentV3PublicConfig = "orisema-update-intent-v3-public-config-prepare"

type UpdateSecretEvidence struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Target   string `json:"target"`
	Identity string `json:"identity"`
}

type UpdateNetworkEvidence struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Identity string `json:"identity"`
	Driver   string `json:"driver"`
}

// UpdateBindingEvidence deliberately has no secret value, public environment
// value, or provider locator field. It is safe to persist in an operation
// journal and proves equality only through metadata and digests.
type UpdateBindingEvidence struct {
	PublicEnvironmentNames  []string                `json:"publicEnvironmentNames,omitempty"`
	PublicEnvironmentDigest string                  `json:"publicEnvironmentDigest,omitempty"`
	Secrets                 []UpdateSecretEvidence  `json:"secrets,omitempty"`
	Networks                []UpdateNetworkEvidence `json:"networks,omitempty"`
}

type UpdateIntent struct {
	Contract                 string                 `json:"contract,omitempty"`
	Service                  string                 `json:"service"`
	CurrentImage             string                 `json:"currentImage"`
	TargetImage              string                 `json:"targetImage"`
	ComparedFields           []string               `json:"comparedFields"`
	PlanFingerprint          string                 `json:"planFingerprint"`
	Bindings                 *UpdateBindingEvidence `json:"bindings,omitempty"`
	ConfigurationFingerprint string                 `json:"configurationFingerprint,omitempty"`
	DifferenceFingerprint    string                 `json:"differenceFingerprint,omitempty"`
	PlannedAt                time.Time              `json:"plannedAt"`
}

type UpdateIntentRequest struct {
	ID           string
	Application  string
	Service      string
	ResourceName string
	ResourceID   string
	CurrentImage string
	TargetImage  string
	Bindings     *UpdateBindingEvidence
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

// PublicConfigurationIntent reports the Beta 1 public-configuration contract
// without exposing its private persisted identifier to CLI callers.
func (op Operation) PublicConfigurationIntent() bool {
	return op.Intent != nil && op.Intent.Contract == updateIntentV3PublicConfig
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

func updatePlanFingerprintV2(application, service, resourceName, resourceID, currentImage, targetImage, configurationFingerprint string) string {
	payload := struct {
		Contract                 string   `json:"contract"`
		Application              string   `json:"application"`
		Service                  string   `json:"service"`
		ResourceName             string   `json:"resourceName"`
		ResourceID               string   `json:"resourceId"`
		CurrentImage             string   `json:"currentImage"`
		TargetImage              string   `json:"targetImage"`
		ComparedFields           []string `json:"comparedFields"`
		ConfigurationFingerprint string   `json:"configurationFingerprint"`
	}{updateIntentV2, application, service, resourceName, resourceID, currentImage, targetImage, updateComparedFields, configurationFingerprint}
	data, _ := json.Marshal(payload)
	return sha256String(data)
}

// UpdatePlanFingerprintWithBindings canonicalizes metadata-only binding
// evidence and returns the v2 plan fingerprint used for fresh revalidation.
// It never accepts secret values, public environment values, or locators.
func UpdatePlanFingerprintWithBindings(application, service, resourceName, resourceID, currentImage, targetImage string, bindings UpdateBindingEvidence) (string, error) {
	_, configurationFingerprint, err := canonicalBindingEvidence(bindings)
	if err != nil {
		return "", err
	}
	return updatePlanFingerprintV2(application, service, resourceName, resourceID, currentImage, targetImage, configurationFingerprint), nil
}

func publicConfigurationPlanFingerprint(application, service, resourceName, resourceID, image, configurationFingerprint, differenceFingerprint string) string {
	payload := struct {
		Contract                 string `json:"contract"`
		Application              string `json:"application"`
		Service                  string `json:"service"`
		ResourceName             string `json:"resourceName"`
		ResourceID               string `json:"resourceId"`
		Image                    string `json:"image"`
		ConfigurationFingerprint string `json:"configurationFingerprint"`
		DifferenceFingerprint    string `json:"differenceFingerprint"`
	}{updateIntentV3PublicConfig, application, service, resourceName, resourceID, image, configurationFingerprint, differenceFingerprint}
	data, _ := json.Marshal(payload)
	return sha256String(data)
}

// PublicConfigurationPlanFingerprint validates value-free public configuration
// evidence and returns the v3 fingerprint used for fresh authorization.
func PublicConfigurationPlanFingerprint(application, service, resourceName, resourceID, image, differenceFingerprint string, bindings UpdateBindingEvidence) (string, error) {
	canonical, configurationFingerprint, err := canonicalBindingEvidence(bindings)
	if err != nil || len(canonical.PublicEnvironmentNames) == 0 || len(canonical.Secrets) != 0 || len(canonical.Networks) != 0 || !isSHA256(differenceFingerprint) {
		return "", errors.New("public configuration authorization evidence is invalid")
	}
	return publicConfigurationPlanFingerprint(application, service, resourceName, resourceID, image, configurationFingerprint, differenceFingerprint), nil
}

func canonicalBindingEvidence(input UpdateBindingEvidence) (UpdateBindingEvidence, string, error) {
	result := UpdateBindingEvidence{
		PublicEnvironmentNames:  append([]string(nil), input.PublicEnvironmentNames...),
		PublicEnvironmentDigest: input.PublicEnvironmentDigest,
		Secrets:                 append([]UpdateSecretEvidence(nil), input.Secrets...),
		Networks:                append([]UpdateNetworkEvidence(nil), input.Networks...),
	}
	sort.Strings(result.PublicEnvironmentNames)
	sort.Slice(result.Secrets, func(i, j int) bool {
		left, right := result.Secrets[i], result.Secrets[j]
		return left.Name+"\x00"+left.Target+"\x00"+left.Provider < right.Name+"\x00"+right.Target+"\x00"+right.Provider
	})
	sort.Slice(result.Networks, func(i, j int) bool {
		left, right := result.Networks[i], result.Networks[j]
		return left.Name+"\x00"+left.Provider < right.Name+"\x00"+right.Provider
	})
	if err := validateBindingEvidence(result); err != nil {
		return UpdateBindingEvidence{}, "", err
	}
	data, _ := json.Marshal(result)
	return result, sha256String(data), nil
}

func validateBindingEvidence(evidence UpdateBindingEvidence) error {
	if (len(evidence.PublicEnvironmentNames) == 0) != (evidence.PublicEnvironmentDigest == "") {
		return errors.New("public environment evidence is incomplete")
	}
	if evidence.PublicEnvironmentDigest != "" && !isSHA256(evidence.PublicEnvironmentDigest) {
		return errors.New("public environment digest is invalid")
	}
	for i, name := range evidence.PublicEnvironmentNames {
		if name == "" || i > 0 && name == evidence.PublicEnvironmentNames[i-1] {
			return errors.New("public environment evidence is empty or duplicated")
		}
	}
	for i, secret := range evidence.Secrets {
		if secret.Name == "" || secret.Provider == "" || secret.Target == "" || secret.Identity == "" {
			return errors.New("secret binding evidence is incomplete")
		}
		if i > 0 && secret.Name == evidence.Secrets[i-1].Name && secret.Target == evidence.Secrets[i-1].Target && secret.Provider == evidence.Secrets[i-1].Provider {
			return errors.New("secret binding evidence is duplicated")
		}
	}
	for i, network := range evidence.Networks {
		if network.Name == "" || network.Provider == "" || network.Identity == "" || network.Driver == "" {
			return errors.New("network binding evidence is incomplete")
		}
		if i > 0 && network.Name == evidence.Networks[i-1].Name && network.Provider == evidence.Networks[i-1].Provider {
			return errors.New("network binding evidence is duplicated")
		}
	}
	return nil
}

func sha256String(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
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
	if request.Bindings != nil {
		bindings, configurationFingerprint, err := canonicalBindingEvidence(*request.Bindings)
		if err != nil {
			return Operation{}, fmt.Errorf("invalid update binding evidence: %w", err)
		}
		intent.Contract = updateIntentV2
		intent.Bindings = &bindings
		intent.ConfigurationFingerprint = configurationFingerprint
		intent.PlanFingerprint = updatePlanFingerprintV2(request.Application, request.Service, request.ResourceName, request.ResourceID, request.CurrentImage, request.TargetImage, configurationFingerprint)
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

// CreatePublicConfigurationIntent persists reviewable value-free evidence for
// public-configuration drift. The contract is intentionally not authorizable
// or executable in this increment.
func (s *Store) CreatePublicConfigurationIntent(request UpdateIntentRequest, differenceFingerprint string) (Operation, error) {
	if request.Bindings == nil || !isSHA256(differenceFingerprint) || request.CurrentImage == "" || request.CurrentImage != request.TargetImage {
		return Operation{}, errors.New("public configuration intent evidence is incomplete")
	}
	bindings, configurationFingerprint, err := canonicalBindingEvidence(*request.Bindings)
	if err != nil || len(bindings.PublicEnvironmentNames) == 0 || len(bindings.Secrets) != 0 || len(bindings.Networks) != 0 {
		return Operation{}, errors.New("public configuration intent contains unsupported binding evidence")
	}
	at := request.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	at = at.UTC()
	intent := &UpdateIntent{
		Contract: updateIntentV3PublicConfig, Service: request.Service,
		CurrentImage: request.CurrentImage, TargetImage: request.TargetImage,
		ComparedFields: append([]string(nil), updateComparedFields...), Bindings: &bindings,
		ConfigurationFingerprint: configurationFingerprint, DifferenceFingerprint: differenceFingerprint, PlannedAt: at,
	}
	intent.PlanFingerprint = publicConfigurationPlanFingerprint(request.Application, request.Service, request.ResourceName, request.ResourceID, request.CurrentImage, configurationFingerprint, differenceFingerprint)
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
	return s.authorizeUpdateIntent(id, planFingerprint, observedResources, at, false)
}

// AuthorizePublicConfigurationIntent advances only a v3 prepare-only intent.
// It records authorization but does not make the intent executable.
func (s *Store) AuthorizePublicConfigurationIntent(id, planFingerprint string, observedResources []Resource, at time.Time) (Operation, error) {
	return s.authorizeUpdateIntent(id, planFingerprint, observedResources, at, true)
}

func (s *Store) authorizeUpdateIntent(id, planFingerprint string, observedResources []Resource, at time.Time, publicConfiguration bool) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Pending || op.Intent == nil || op.Authorization != nil {
			return errors.New("update intent is not pending authorization")
		}
		if op.Intent.Contract == updateIntentV3PublicConfig && !publicConfiguration {
			return errors.New("public configuration intent is review-only and cannot be authorized")
		}
		if op.Intent.Contract != updateIntentV3PublicConfig && publicConfiguration {
			return errors.New("update intent is not a public configuration intent")
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
		detail := "plan and resource evidence revalidated"
		if publicConfiguration {
			detail = "public configuration plan and resource evidence revalidated; runtime execution remains blocked"
		}
		op.Events = append(op.Events, Event{At: at, From: Pending, To: Preparing, Step: "update-authorized", Detail: detail})
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
	publicConfigPrepare := intent.Contract == updateIntentV3PublicConfig
	if publicConfigPrepare {
		validState := op.State == Pending && op.Authorization == nil && op.Execution == nil ||
			op.State == Preparing && op.Authorization != nil && op.Execution == nil ||
			op.State == Applying && op.Authorization != nil && op.Execution != nil && op.Health == nil && op.Decision == nil ||
			op.State == Verifying && op.Authorization != nil && op.Execution != nil && op.Health != nil && op.Decision == nil ||
			(op.State == Completed || op.State == RecoveryRequired) && op.Authorization != nil && op.Execution != nil && op.Health != nil && op.Decision != nil && op.Rollback == nil ||
			(op.State == RollingBack || op.State == RolledBack) && op.Authorization != nil && op.Execution != nil && op.Health != nil && op.Decision != nil && op.Rollback != nil
		if !validState {
			return errors.New("persisted public configuration intent has invalid lifecycle evidence")
		}
	}
	if op.Type != "UPDATE" || intent.Service == "" || intent.CurrentImage == "" || intent.TargetImage == "" || (!publicConfigPrepare && intent.CurrentImage == intent.TargetImage) || (publicConfigPrepare && intent.CurrentImage != intent.TargetImage) {
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
	if intent.Contract != "" {
		if intent.Contract != updateIntentV2 && intent.Contract != updateIntentV3PublicConfig || intent.Bindings == nil || !isSHA256(intent.ConfigurationFingerprint) {
			return errors.New("persisted update intent binding contract is invalid")
		}
		bindings, configurationFingerprint, err := canonicalBindingEvidence(*intent.Bindings)
		if err != nil || !reflect.DeepEqual(bindings, *intent.Bindings) || configurationFingerprint != intent.ConfigurationFingerprint {
			return errors.New("persisted update intent binding evidence is invalid")
		}
		if intent.Contract == updateIntentV3PublicConfig {
			if len(bindings.PublicEnvironmentNames) == 0 || len(bindings.Secrets) != 0 || len(bindings.Networks) != 0 || !isSHA256(intent.DifferenceFingerprint) {
				return errors.New("persisted public configuration intent evidence is invalid")
			}
			want = publicConfigurationPlanFingerprint(op.Application, intent.Service, resource.Name, resource.ID, intent.CurrentImage, configurationFingerprint, intent.DifferenceFingerprint)
		} else {
			if intent.DifferenceFingerprint != "" {
				return errors.New("persisted update intent contains unexpected difference evidence")
			}
			want = updatePlanFingerprintV2(op.Application, intent.Service, resource.Name, resource.ID, intent.CurrentImage, intent.TargetImage, configurationFingerprint)
		}
	} else if intent.Bindings != nil || intent.ConfigurationFingerprint != "" {
		return errors.New("persisted legacy update intent contains binding evidence")
	}
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
