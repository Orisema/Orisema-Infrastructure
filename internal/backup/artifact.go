// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/Orisema/Orisema-Infrastructure/internal/buildinfo"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/operation"
)

const (
	Format        = "orisema.backup/v1alpha1"
	maxManifest   = 1 << 20
	maxOperations = 32 << 20
	maxTotal      = 64 << 20
)

type Entry struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}
type Scope struct {
	Configuration    bool `json:"configuration"`
	OperationHistory bool `json:"operationHistory"`
}
type Metadata struct {
	Format         string    `json:"format"`
	BackupID       string    `json:"backupId"`
	Application    string    `json:"application"`
	CreatedAt      time.Time `json:"createdAt"`
	OrisemaVersion string    `json:"orisemaVersion"`
	Scope          Scope     `json:"scope"`
	Exclusions     []string  `json:"exclusions"`
	Entries        []Entry   `json:"entries"`
}
type Verified struct {
	Metadata Metadata
	Payload  map[string][]byte
}
type CreateRequest struct{ ManifestPath, ArtifactPath, JournalPath, OperationsPath string }
type RestoreRequest struct{ ArtifactPath, DestinationPath, JournalPath string }

func Create(r CreateRequest) (Metadata, error) {
	manifestBytes, err := readSourceRegular(r.ManifestPath, maxManifest)
	if err != nil {
		return Metadata{}, fmt.Errorf("read manifest source: %w", err)
	}
	m, err := manifestFromBytes(manifestBytes)
	if err != nil {
		return Metadata{}, err
	}
	payload := map[string][]byte{"manifest.yaml": manifestBytes}
	if r.OperationsPath != "" {
		history, err := exportOperations(r.OperationsPath, m.Name)
		if err != nil {
			return Metadata{}, err
		}
		payload["operations.jsonl"] = history
	}
	id, err := randomID("backup-")
	if err != nil {
		return Metadata{}, err
	}
	meta := Metadata{Format: Format, BackupID: id, Application: m.Name, CreatedAt: time.Now().UTC(), OrisemaVersion: buildinfo.Current().Version,
		Scope: Scope{Configuration: true, OperationHistory: r.OperationsPath != ""}, Exclusions: []string{"credentials", "persistentData", "runtimeState", "secrets"}}
	for name, data := range payload {
		sum := sha256.Sum256(data)
		meta.Entries = append(meta.Entries, Entry{Name: name, MediaType: mediaType(name), Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(meta.Entries, func(i, j int) bool { return meta.Entries[i].Name < meta.Entries[j].Name })
	if err := requireAbsent(r.ArtifactPath); err != nil {
		return Metadata{}, err
	}
	parent, err := secureParent(r.ArtifactPath)
	if err != nil {
		return Metadata{}, err
	}
	store, err := operation.OpenStore(r.JournalPath)
	if err != nil {
		return Metadata{}, err
	}
	opID, err := randomID("backup-op-")
	if err != nil {
		return Metadata{}, err
	}
	stage := r.ArtifactPath + ".staging-" + strings.TrimPrefix(opID, "backup-op-")
	manifestSum := sha256.Sum256(manifestBytes)
	if _, err := store.CreateFileOperation(operation.FileOperationRequest{ID: opID, Type: "BACKUP", Application: m.Name, BackupID: id, Source: r.ManifestPath, SourceSHA256: hex.EncodeToString(manifestSum[:]), Artifact: r.ArtifactPath, Staging: stage, FirstStep: "stage"}); err != nil {
		return Metadata{}, err
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		return Metadata{}, fmt.Errorf("create backup staging: %w", err)
	}
	if _, err := store.RecordFileOperationStep(opID, "stage", "payload", time.Time{}); err != nil {
		return Metadata{}, err
	}
	if err := os.Mkdir(filepath.Join(stage, "payload"), 0o700); err != nil {
		return Metadata{}, err
	}
	for _, entry := range meta.Entries {
		if err := writePrivate(filepath.Join(stage, "payload", entry.Name), payload[entry.Name]); err != nil {
			return Metadata{}, err
		}
	}
	if err := syncDir(filepath.Join(stage, "payload")); err != nil {
		return Metadata{}, err
	}
	if _, err := store.RecordFileOperationStep(opID, "payload", "metadata", time.Time{}); err != nil {
		return Metadata{}, err
	}
	metadataBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Metadata{}, err
	}
	metadataBytes = append(metadataBytes, '\n')
	if err := writePrivate(filepath.Join(stage, "metadata.json"), metadataBytes); err != nil {
		return Metadata{}, err
	}
	sum := sha256.Sum256(metadataBytes)
	if err := writePrivate(filepath.Join(stage, "metadata.sha256"), []byte(hex.EncodeToString(sum[:])+"\n")); err != nil {
		return Metadata{}, err
	}
	if err := syncDir(stage); err != nil {
		return Metadata{}, err
	}
	if _, err := store.RecordFileOperationStep(opID, "metadata", "publish", time.Time{}); err != nil {
		return Metadata{}, err
	}
	if err := requireAbsent(r.ArtifactPath); err != nil {
		return Metadata{}, err
	}
	if err := renameNoReplace(stage, r.ArtifactPath); err != nil {
		return Metadata{}, fmt.Errorf("publish backup: %w", err)
	}
	if err := syncDir(parent); err != nil {
		return Metadata{}, err
	}
	if _, err := store.RecordFileOperationStep(opID, "publish", "verify", time.Time{}); err != nil {
		return Metadata{}, err
	}
	if _, err := Verify(r.ArtifactPath); err != nil {
		return Metadata{}, fmt.Errorf("verify published backup: %w", err)
	}
	if _, err := store.RecordFileOperationStep(opID, "verify", "", time.Time{}); err != nil {
		return Metadata{}, err
	}
	return meta, nil
}

func Verify(root string) (Verified, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return Verified{}, errors.New("backup root must be a private real directory")
	}
	allowed := map[string]bool{"metadata.json": true, "metadata.sha256": true, "payload": true}
	entries, err := os.ReadDir(root)
	if err != nil {
		return Verified{}, err
	}
	for _, e := range entries {
		if !allowed[e.Name()] {
			return Verified{}, fmt.Errorf("unexpected backup entry %q", e.Name())
		}
	}
	metadataBytes, err := readPrivate(filepath.Join(root, "metadata.json"), 1<<20)
	if err != nil {
		return Verified{}, err
	}
	checksumBytes, err := readPrivate(filepath.Join(root, "metadata.sha256"), 128)
	if err != nil {
		return Verified{}, err
	}
	want := strings.TrimSpace(string(checksumBytes))
	if len(want) != 64 || want != strings.ToLower(want) {
		return Verified{}, errors.New("metadata checksum is invalid")
	}
	if decoded, err := hex.DecodeString(want); err != nil || len(decoded) != sha256.Size {
		return Verified{}, errors.New("metadata checksum is invalid")
	}
	sum := sha256.Sum256(metadataBytes)
	if !strings.EqualFold(want, hex.EncodeToString(sum[:])) {
		return Verified{}, errors.New("metadata checksum mismatch")
	}
	if err := rejectDuplicateJSON(metadataBytes); err != nil {
		return Verified{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(metadataBytes))
	dec.DisallowUnknownFields()
	var meta Metadata
	if err := dec.Decode(&meta); err != nil {
		return Verified{}, fmt.Errorf("decode metadata: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return Verified{}, errors.New("metadata has trailing content")
	}
	if err := validateMetadata(meta); err != nil {
		return Verified{}, err
	}
	payloadDir := filepath.Join(root, "payload")
	pi, err := os.Lstat(payloadDir)
	if err != nil || !pi.IsDir() || pi.Mode()&os.ModeSymlink != 0 || pi.Mode().Perm() != 0o700 {
		return Verified{}, errors.New("payload must be a private real directory")
	}
	payloadEntries, err := os.ReadDir(payloadDir)
	if err != nil {
		return Verified{}, err
	}
	if len(payloadEntries) != len(meta.Entries) {
		return Verified{}, errors.New("payload inventory count mismatch")
	}
	result := Verified{Metadata: meta, Payload: map[string][]byte{}}
	var total int64
	for _, entry := range meta.Entries {
		data, err := readPrivate(filepath.Join(payloadDir, entry.Name), limitFor(entry.Name))
		if err != nil {
			return Verified{}, err
		}
		total += int64(len(data))
		if total > maxTotal {
			return Verified{}, errors.New("backup exceeds total size limit")
		}
		sum := sha256.Sum256(data)
		if int64(len(data)) != entry.Bytes || hex.EncodeToString(sum[:]) != entry.SHA256 {
			return Verified{}, fmt.Errorf("payload %s integrity mismatch", entry.Name)
		}
		result.Payload[entry.Name] = data
	}
	for _, entry := range payloadEntries {
		if _, ok := result.Payload[entry.Name()]; !ok {
			return Verified{}, fmt.Errorf("unexpected payload %q", entry.Name())
		}
	}
	loaded, err := manifestFromBytes(result.Payload["manifest.yaml"])
	if err != nil {
		return Verified{}, fmt.Errorf("backup manifest is invalid: %w", err)
	}
	if loaded.Name != meta.Application {
		return Verified{}, errors.New("backup application does not match manifest")
	}
	if history, ok := result.Payload["operations.jsonl"]; ok {
		if err := validateOperations(history, meta.Application); err != nil {
			return Verified{}, err
		}
	}
	return result, nil
}

func Restore(r RestoreRequest) (Metadata, error) {
	verified, err := Verify(r.ArtifactPath)
	if err != nil {
		return Metadata{}, err
	}
	if err := requireAbsent(r.DestinationPath); err != nil {
		return Metadata{}, err
	}
	parent, err := secureParent(r.DestinationPath)
	if err != nil {
		return Metadata{}, err
	}
	store, err := operation.OpenStore(r.JournalPath)
	if err != nil {
		return Metadata{}, err
	}
	opID, err := randomID("restore-op-")
	if err != nil {
		return Metadata{}, err
	}
	stage := r.DestinationPath + ".staging-" + strings.TrimPrefix(opID, "restore-op-")
	if _, err := store.CreateFileOperation(operation.FileOperationRequest{ID: opID, Type: "RESTORE", Application: verified.Metadata.Application, BackupID: verified.Metadata.BackupID, Source: r.ArtifactPath, Artifact: r.ArtifactPath, Destination: r.DestinationPath, Staging: stage, FirstStep: "stage"}); err != nil {
		return Metadata{}, err
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		return Metadata{}, err
	}
	if _, err := store.RecordFileOperationStep(opID, "stage", "payload", time.Time{}); err != nil {
		return Metadata{}, err
	}
	for _, entry := range verified.Metadata.Entries {
		if err := writePrivate(filepath.Join(stage, entry.Name), verified.Payload[entry.Name]); err != nil {
			return Metadata{}, err
		}
	}
	if err := syncDir(stage); err != nil {
		return Metadata{}, err
	}
	if _, err := store.RecordFileOperationStep(opID, "payload", "publish", time.Time{}); err != nil {
		return Metadata{}, err
	}
	if err := requireAbsent(r.DestinationPath); err != nil {
		return Metadata{}, err
	}
	if err := renameNoReplace(stage, r.DestinationPath); err != nil {
		return Metadata{}, err
	}
	if err := syncDir(parent); err != nil {
		return Metadata{}, err
	}
	if _, err := store.RecordFileOperationStep(opID, "publish", "verify", time.Time{}); err != nil {
		return Metadata{}, err
	}
	for _, entry := range verified.Metadata.Entries {
		data, err := readPrivate(filepath.Join(r.DestinationPath, entry.Name), limitFor(entry.Name))
		if err != nil {
			return Metadata{}, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != entry.SHA256 {
			return Metadata{}, errors.New("restored payload verification failed")
		}
	}
	if _, err := store.RecordFileOperationStep(opID, "verify", "", time.Time{}); err != nil {
		return Metadata{}, err
	}
	return verified.Metadata, nil
}

func validateMetadata(m Metadata) error {
	if m.Format != Format || !regexp.MustCompile(`^backup-[0-9a-f]{32}$`).MatchString(m.BackupID) || m.Application == "" || m.CreatedAt.IsZero() || m.OrisemaVersion == "" || !m.Scope.Configuration {
		return errors.New("unsupported or incomplete backup metadata")
	}
	if len(m.Entries) < 1 || len(m.Entries) > 64 {
		return errors.New("backup entry count is invalid")
	}
	if !equalStrings(m.Exclusions, []string{"credentials", "persistentData", "runtimeState", "secrets"}) {
		return errors.New("backup exclusions are invalid")
	}
	seen := map[string]bool{}
	previous := ""
	for _, e := range m.Entries {
		decoded, hashErr := hex.DecodeString(e.SHA256)
		if (e.Name != "manifest.yaml" && e.Name != "operations.jsonl") || seen[e.Name] || e.Name <= previous || e.MediaType != mediaType(e.Name) || e.Bytes < 0 || e.Bytes > int64(limitFor(e.Name)) || hashErr != nil || len(decoded) != sha256.Size || e.SHA256 != strings.ToLower(e.SHA256) {
			return errors.New("backup inventory is invalid")
		}
		seen[e.Name] = true
		previous = e.Name
	}
	if !seen["manifest.yaml"] || m.Scope.OperationHistory != seen["operations.jsonl"] {
		return errors.New("backup scope does not match inventory")
	}
	return nil
}
func mediaType(name string) string {
	if name == "manifest.yaml" {
		return "application/yaml"
	}
	return "application/x-ndjson"
}
func limitFor(name string) int64 {
	if name == "manifest.yaml" {
		return maxManifest
	}
	if name == "operations.jsonl" {
		return maxOperations
	}
	return 0
}
func randomID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
func requireAbsent(path string) error {
	if path == "" {
		return errors.New("destination is required")
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
func secureParent(path string) (string, error) {
	p := filepath.Dir(path)
	i, err := os.Lstat(p)
	if err != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("destination parent must be a real directory")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	if abs != resolvedAbs {
		return "", errors.New("destination parent path must not traverse symlinks")
	}
	return p, nil
}
func writePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err != nil {
		return err
	}
	return cerr
}
func readSourceRegular(path string, limit int64) ([]byte, error) {
	return readRegular(path, limit, false)
}
func readPrivate(path string, limit int64) ([]byte, error) {
	return readRegular(path, limit, true)
}
func readRegular(path string, limit int64, private bool) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open regular file %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || (private && before.Mode().Perm() != 0o600) {
		return nil, fmt.Errorf("%s must be a %sregular file", filepath.Base(path), map[bool]string{true: "private ", false: ""}[private])
	}
	if before.Size() > limit {
		return nil, fmt.Errorf("%s exceeds size limit", filepath.Base(path))
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds size limit", filepath.Base(path))
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() != int64(len(data)) || after.ModTime() != before.ModTime() {
		return nil, errors.New("source changed while being read")
	}
	return data, nil
}
func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func manifestFromBytes(data []byte) (*manifest.Manifest, error) {
	dir, err := os.MkdirTemp("", "orisema-backup-verify-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return nil, err
	}
	return manifest.Load(p)
}
func exportOperations(root, app string) ([]byte, error) {
	store, err := operation.OpenExistingStore(root)
	if err != nil {
		return nil, err
	}
	ops, err := store.List()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	for _, op := range ops {
		if op.Application != app {
			continue
		}
		line, err := json.Marshal(op)
		if err != nil {
			return nil, err
		}
		if out.Len()+len(line)+1 > maxOperations {
			return nil, errors.New("operation history exceeds size limit")
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func validateOperations(data []byte, app string) error {
	for len(data) != 0 {
		index := bytes.IndexByte(data, '\n')
		if index < 0 {
			return errors.New("operation history is not newline terminated")
		}
		line := data[:index]
		data = data[index+1:]
		if len(line) == 0 {
			return errors.New("operation history contains an empty record")
		}
		var op operation.Operation
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&op); err != nil {
			return fmt.Errorf("invalid operation history: %w", err)
		}
		if dec.Decode(&struct{}{}) != io.EOF || op.Application != app || operation.ValidateSnapshot(op) != nil {
			return errors.New("operation history contains an invalid or foreign snapshot")
		}
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func rejectDuplicateJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var value func() error
	value = func() error {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("metadata object key is invalid")
				}
				if seen[key] {
					return fmt.Errorf("metadata contains duplicate key %q", key)
				}
				seen[key] = true
				if err := value(); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim('}') {
				return errors.New("metadata object is malformed")
			}
		case '[':
			for dec.More() {
				if err := value(); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim(']') {
				return errors.New("metadata array is malformed")
			}
		default:
			return errors.New("metadata delimiter is invalid")
		}
		return nil
	}
	if err := value(); err != nil {
		return fmt.Errorf("invalid metadata JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("metadata has trailing content")
	}
	return nil
}
