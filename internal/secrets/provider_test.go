// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type fakeInspector struct{}

func (fakeInspector) InspectMetadata(_ context.Context, ref Reference) (Metadata, error) {
	return Metadata{Provider: ref.Provider, Locator: ref.Locator, Identity: "external-id"}, nil
}

func TestProviderContractContainsMetadataOnly(t *testing.T) {
	var inspector Inspector = fakeInspector{}
	got, err := inspector.InspectMetadata(context.Background(), Reference{Provider: ProviderPodman, Locator: "database-password"})
	if err != nil || got.Identity != "external-id" {
		t.Fatalf("InspectMetadata() = %#v, %v", got, err)
	}
	for _, value := range []any{Reference{}, Metadata{}} {
		typeOf := reflect.TypeOf(value)
		for i := 0; i < typeOf.NumField(); i++ {
			name := strings.ToLower(typeOf.Field(i).Name)
			if strings.Contains(name, "value") || strings.Contains(name, "content") || strings.Contains(name, "data") || strings.Contains(name, "hash") {
				t.Fatalf("metadata-only type %s has forbidden field %q", typeOf.Name(), typeOf.Field(i).Name)
			}
		}
	}
}
