// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// DifferenceKind is a closed, value-free classification of managed drift.
type DifferenceKind string

const (
	DifferenceImage          DifferenceKind = "IMAGE"
	DifferencePublicConfig   DifferenceKind = "PUBLIC_CONFIG"
	DifferenceSecretBinding  DifferenceKind = "SECRET_BINDING"
	DifferenceNetworkBinding DifferenceKind = "NETWORK_BINDING"
	DifferencePort           DifferenceKind = "PORT"
	DifferenceHealthcheck    DifferenceKind = "HEALTHCHECK"
	DifferenceRestartPolicy  DifferenceKind = "RESTART_POLICY"
	DifferenceUnknown        DifferenceKind = "UNKNOWN"
)

// Difference contains classification metadata only. Detail is deliberately a
// stable diagnostic phrase; it must never contain a secret value or locator.
type Difference struct {
	Kind   DifferenceKind `json:"kind"`
	Detail string         `json:"detail"`
}

func classifyDifferences(details []string) []Difference {
	result := make([]Difference, 0, len(details))
	for _, detail := range details {
		kind := DifferenceUnknown
		typedDetail := detail
		switch detail {
		case "configured image differs":
			kind = DifferenceImage
		case "public configuration metadata differs", "public configuration differs", "unexpected public configuration metadata":
			kind = DifferencePublicConfig
		case "secret binding metadata differs", "unexpected secret binding metadata":
			kind = DifferenceSecretBinding
		case "network binding metadata differs", "unexpected network binding metadata", "network attachment count differs", "network attachment differs":
			kind = DifferenceNetworkBinding
		case "published port count differs", "published port binding differs":
			kind = DifferencePort
		case "healthcheck missing", "healthcheck configuration differs", "unexpected healthcheck configured":
			kind = DifferenceHealthcheck
		case "restart policy differs":
			kind = DifferenceRestartPolicy
		default:
			typedDetail = "unsupported managed difference"
		}
		result = append(result, Difference{Kind: kind, Detail: typedDetail})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			return result[i].Detail < result[j].Detail
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

// DifferenceFingerprint returns a deterministic digest of value-free drift
// metadata. Unknown kinds remain fingerprintable but are never eligible.
func DifferenceFingerprint(differences []Difference) string {
	canonical := append([]Difference(nil), differences...)
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Kind == canonical[j].Kind {
			return canonical[i].Detail < canonical[j].Detail
		}
		return canonical[i].Kind < canonical[j].Kind
	})
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
