// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package publicconfig derives non-secret metadata for explicitly public
// environment entries. Values remain observable through the container runtime.
package publicconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

const (
	NamesLabel = "dev.orisema.public-env.names"
	HashLabel  = "dev.orisema.public-env.hash"
)

// Metadata returns a stable managed-name list and a digest of the declared
// name/value pairs. It is drift metadata, not a confidentiality mechanism.
func Metadata(entries []manifest.PublicEnvironmentEntry) (string, string) {
	ordered := append([]manifest.PublicEnvironmentEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	names := make([]string, 0, len(ordered))
	hash := sha256.New()
	for _, entry := range ordered {
		names = append(names, entry.Name)
		writeFramed(hash, entry.Name)
		writeFramed(hash, entry.Value)
	}
	return strings.Join(names, ","), hex.EncodeToString(hash.Sum(nil))
}

type writer interface {
	Write([]byte) (int, error)
}

func writeFramed(dst writer, value string) {
	_, _ = dst.Write([]byte(strconv.Itoa(len(value))))
	_, _ = dst.Write([]byte{':'})
	_, _ = dst.Write([]byte(value))
	_, _ = dst.Write([]byte{0})
}

// EnvironmentMap applies the runtime's last-entry-wins representation without
// returning malformed entries.
func EnvironmentMap(entries []string) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" {
			result[name] = value
		}
	}
	return result
}
