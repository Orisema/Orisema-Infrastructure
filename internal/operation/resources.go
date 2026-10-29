// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
	"fmt"
	"sort"
)

type Ownership string

const (
	OwnershipManaged  Ownership = "managed"
	OwnershipExternal Ownership = "external"
	OwnershipUnknown  Ownership = "unknown"
)

type ResourceEvidence struct {
	Safe     bool
	Managed  int
	External int
	Issues   []string
}

// RevalidateResources compares a recorded resource set with a fresh
// observation. It is pure and cannot query or mutate a runtime.
func RevalidateResources(expected, observed []Resource) ResourceEvidence {
	result := ResourceEvidence{}
	expectedByKey, issues := indexResources(expected, "recorded")
	observedByKey, observedIssues := indexResources(observed, "observed")
	result.Issues = append(issues, observedIssues...)
	keys := make([]string, 0, len(expectedByKey))
	for key := range expectedByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		want := expectedByKey[key]
		if want.Ownership == OwnershipManaged {
			result.Managed++
		} else if want.Ownership == OwnershipExternal {
			result.External++
		} else {
			result.Issues = append(result.Issues, fmt.Sprintf("%s has unknown recorded ownership", key))
			continue
		}
		got, ok := observedByKey[key]
		if !ok {
			result.Issues = append(result.Issues, fmt.Sprintf("%s is missing from runtime evidence", key))
			continue
		}
		if got.Ownership != want.Ownership {
			result.Issues = append(result.Issues, fmt.Sprintf("%s ownership changed from %s to %s", key, want.Ownership, got.Ownership))
		}
		if got.ID != want.ID {
			result.Issues = append(result.Issues, fmt.Sprintf("%s identity changed", key))
		}
		if want.Ownership == OwnershipManaged && (got.Application != want.Application || got.Service != want.Service) {
			result.Issues = append(result.Issues, fmt.Sprintf("%s managed scope changed", key))
		}
	}
	result.Safe = len(result.Issues) == 0
	return result
}

func ResourceEvidenceComplete(resources []Resource) (complete bool, managed, external int) {
	if len(resources) == 0 || validateResources(resources) != nil {
		return false, 0, 0
	}
	for _, resource := range resources {
		switch resource.Ownership {
		case OwnershipManaged:
			managed++
		case OwnershipExternal:
			external++
		default:
			return false, managed, external
		}
	}
	return managed > 0, managed, external
}

func validateResources(resources []Resource) error {
	_, issues := indexResources(resources, "recorded")
	if len(issues) != 0 {
		return errors.New(issues[0])
	}
	return nil
}

func indexResources(resources []Resource, source string) (map[string]Resource, []string) {
	indexed := make(map[string]Resource, len(resources))
	var issues []string
	for _, resource := range resources {
		key := resource.Kind + "/" + resource.Name
		if resource.Kind == "" || resource.Name == "" {
			issues = append(issues, source+" resource kind and name are required")
			continue
		}
		if _, exists := indexed[key]; exists {
			issues = append(issues, fmt.Sprintf("duplicate %s resource %s", source, key))
			continue
		}
		switch resource.Ownership {
		case OwnershipManaged:
			if resource.ID == "" || resource.Application == "" || resource.Service == "" {
				issues = append(issues, fmt.Sprintf("managed %s resource %s requires ID, application and service", source, key))
			}
		case OwnershipExternal:
			if resource.ID == "" || resource.Application != "" || resource.Service != "" {
				issues = append(issues, fmt.Sprintf("external %s resource %s requires ID and no managed scope", source, key))
			}
		case OwnershipUnknown:
			// Unknown is representable so discovery can be journaled, but it is
			// never sufficient for recovery.
		default:
			issues = append(issues, fmt.Sprintf("%s resource %s has unsupported ownership %q", source, key, resource.Ownership))
		}
		indexed[key] = resource
	}
	return indexed, issues
}
